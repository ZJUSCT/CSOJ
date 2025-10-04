package judger

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"go.uber.org/zap"
)

type DockerManager struct {
	cli *client.Client
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func NewDockerManager(host string) (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerManager{cli: cli}, nil
}

func (m *DockerManager) CreateContainer(image, workDir string, cpu int, memory int64, asRoot bool) (string, error) {
	ctx := context.Background()

	config := &container.Config{
		Image:           image,
		WorkingDir:      "/mnt/work",
		Tty:             false, // Tty must be false to multiplex stdout/stderr
		OpenStdin:       true,
		AttachStdin:     true,
		AttachStdout:    true,
		AttachStderr:    true,
		NetworkDisabled: true,
	}

	if !asRoot {
		config.User = "1000:1000"
	}

	hostConfig := &container.HostConfig{
		Binds: []string{workDir + ":/mnt/work"},
		Resources: container.Resources{
			NanoCPUs: int64(cpu) * 1e9,
			Memory:   memory * 1024 * 1024,
		},
	}

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (m *DockerManager) StartContainer(containerID string) error {
	return m.cli.ContainerStart(context.Background(), containerID, container.StartOptions{})
}

// stream-oriented
func (m *DockerManager) ExecInContainer(containerID string, cmd []string, timeout time.Duration, outputCallback func(streamType string, data []byte)) (ExecResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execCreateResp, err := m.cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return ExecResult{}, err
	}
	execID := execCreateResp.ID

	resp, err := m.cli.ContainerExecAttach(ctx, execID, container.ExecAttachOptions{})
	if err != nil {
		return ExecResult{}, err
	}
	defer resp.Close()

	var stdoutBuf, stderrBuf bytes.Buffer

	// Use a custom writer for the callback
	stdoutWriter := newCallbackWriter("stdout", &stdoutBuf, outputCallback)
	stderrWriter := newCallbackWriter("stderr", &stderrBuf, outputCallback)

	// stdcopy.StdCopy demultiplexes the stream from Docker into separate stdout and stderr
	_, err = stdcopy.StdCopy(stdoutWriter, stderrWriter, resp.Reader)
	if err != nil {
		zap.S().Warnf("error copying stdout/stderr from container exec: %v", err)
	}

	var inspect container.ExecInspect
	for {
		inspect, err = m.cli.ContainerExecInspect(ctx, execID)
		if err != nil {
			return ExecResult{}, err
		}
		if !inspect.Running {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	return ExecResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: inspect.ExitCode,
	}, nil
}

// an io.Writer that calls a callback function and writes to a buffer.
type callbackWriter struct {
	streamType string
	buffer     *bytes.Buffer
	callback   func(streamType string, data []byte)
}

func newCallbackWriter(streamType string, buffer *bytes.Buffer, callback func(string, []byte)) *callbackWriter {
	return &callbackWriter{
		streamType: streamType,
		buffer:     buffer,
		callback:   callback,
	}
}

func (w *callbackWriter) Write(p []byte) (int, error) {
	// To avoid sending sensitive data or large binary chunks over websocket,
	// we marshal the raw bytes into a JSON-safe string.
	// A simple string conversion is okay for typical text output.
	// For more complex data, base64 encoding might be better.
	jsonBytes, err := json.Marshal(string(p))
	if err != nil {
		return 0, err
	}
	// The jsonBytes will be like `"hello\nworld"` (including quotes)
	// We trim the quotes for cleaner data on the client side.
	cleanBytes := bytes.Trim(jsonBytes, `"`)

	w.callback(w.streamType, cleanBytes)
	return w.buffer.Write(p)
}

func (m *DockerManager) CleanupContainer(containerID string) {
	ctx := context.Background()

	_, err := m.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		// Container might already be removed
		zap.S().Warnf("failed to inspect container %s before cleanup: %v", containerID, err)
		return
	}

	timeoutSeconds := 5
	stopOptions := container.StopOptions{Timeout: &timeoutSeconds}
	if err := m.cli.ContainerStop(ctx, containerID, stopOptions); err != nil {
		zap.S().Warnf("failed to stop container %s: %v", containerID, err)
	}

	removeOptions := container.RemoveOptions{Force: true}
	if err := m.cli.ContainerRemove(ctx, containerID, removeOptions); err != nil {
		zap.S().Warnf("failed to remove container %s: %v", containerID, err)
		return
	}

	zap.S().Infof("cleaned up container %s", containerID)
}

func (m *DockerManager) CopyToContainer(containerID string, srcDir string, dstDir string) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		fr, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fr.Close()

		hdr := &tar.Header{
			Name: relPath,
			Mode: 0644,
			Size: info.Size(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := io.Copy(tw, fr); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk source directory: %w", err)
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	tarReader := bytes.NewReader(buf.Bytes())
	return m.cli.CopyToContainer(context.Background(), containerID, dstDir, tarReader, container.CopyToContainerOptions{})
}
