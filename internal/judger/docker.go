package judger

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ZJUSCT/CSOJ/internal/config"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
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

func NewDockerManager(cfg config.DockerConfig) (*DockerManager, error) {
	opts := []client.Opt{
		client.WithHost(cfg.Host),
		client.WithAPIVersionNegotiation(),
	}

	if cfg.TLSVerify {
		// client.WithTLSClientConfig can create the http.Client with TLS config
		// from the given paths.
		tlsOpts := client.WithTLSClientConfig(cfg.CACert, cfg.Cert, cfg.Key)
		opts = append(opts, tlsOpts)
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}
	return &DockerManager{cli: cli}, nil
}

func (m *DockerManager) CreateContainer(image, workDir string, cpu int, cpusetCpus string, memory int64, asRoot bool, mounts []Mount, networkEnabled bool, envs []string) (string, error) {
	ctx := context.Background()

	config := &container.Config{
		Image:           image,
		Tty:             false, // Tty must be false to multiplex stdout/stderr
		OpenStdin:       true,
		AttachStdin:     true,
		AttachStdout:    true,
		AttachStderr:    true,
		NetworkDisabled: !networkEnabled,
		Env:             envs,
	}

	if !asRoot {
		config.User = "1000:1000"
	}

	hostConfig := &container.HostConfig{
		Binds: []string{workDir + ":/mnt/work"},
		Resources: container.Resources{
			NanoCPUs:   int64(cpu) * 1e9,
			Memory:     memory * 1024 * 1024,
			CpusetCpus: cpusetCpus,
		},
	}

	dockerMounts := []mount.Mount{}
	for _, mnt := range mounts {
		mountType := mount.TypeBind
		if mnt.Type != "" {
			mountType = mount.Type(mnt.Type)
		}

		isReadOnly := true // Default to true
		if mnt.ReadOnly != nil {
			isReadOnly = *mnt.ReadOnly
		}

		dockerMounts = append(dockerMounts, mount.Mount{
			Type:     mountType,
			Source:   mnt.Source,
			Target:   mnt.Target,
			ReadOnly: isReadOnly,
			TmpfsOptions: &mount.TmpfsOptions{
				SizeBytes: mnt.TmpfsOption.SizeBytes,
				Mode:      mnt.TmpfsOption.Mode,
				Options:   mnt.TmpfsOption.Options,
			},
		})
	}
	hostConfig.Mounts = dockerMounts

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (m *DockerManager) StartContainer(containerID string) error {
	return m.cli.ContainerStart(context.Background(), containerID, container.StartOptions{})
}

func (m *DockerManager) ExecInContainer(ctx context.Context, containerID string, cmd []string, outputCallback func(streamType string, data []byte)) (ExecResult, error) {
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
	stdoutWriter := newCallbackWriter("stdout", &stdoutBuf, outputCallback)
	stderrWriter := newCallbackWriter("stderr", &stderrBuf, outputCallback)

	_, err = stdcopy.StdCopy(stdoutWriter, stderrWriter, resp.Reader)
	if err != nil {
		zap.S().Warnf("error copying stdout/stderr from container exec: %v", err)
	}

	var inspect container.ExecInspect
	for {
		if ctx.Err() != nil {
			return ExecResult{}, ctx.Err()
		}

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
	w.callback(w.streamType, p)
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

	timeoutSeconds := 0
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
