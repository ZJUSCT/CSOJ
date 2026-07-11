package devpods

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ZJUSCT/CSOJ/internal/config"
)

// Gateway is the stored shape of the `devpods.gateway` setting.
type Gateway struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	HostnameSuffix string `json:"hostname_suffix,omitempty"`
}

var hostnameSuffixRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Validate checks that a gateway can produce a usable SSH command.
func (g Gateway) Validate() error {
	if strings.TrimSpace(g.Host) == "" {
		return fmt.Errorf("gateway host is required")
	}
	if g.Port < 1 || g.Port > 65535 {
		return fmt.Errorf("gateway port must be between 1 and 65535")
	}
	if g.HostnameSuffix != "" && !hostnameSuffixRE.MatchString(g.HostnameSuffix) {
		return fmt.Errorf("gateway hostname suffix must be a lowercase DNS label")
	}
	return nil
}

const gatewaySettingKey = "devpods.gateway"

// GetGateway reads the devpods gateway setting. Returns a zero value
// (no error) when the setting is absent.
func GetGateway(s *config.SettingsStore) (Gateway, error) {
	var g Gateway
	if err := s.Get(gatewaySettingKey, &g); err != nil {
		return Gateway{}, err
	}
	return g, nil
}

// SetGateway writes the devpods gateway setting.
func SetGateway(s *config.SettingsStore, g Gateway) error {
	return s.Set(gatewaySettingKey, g)
}

// SSHCommand builds the user-facing login string. The gateway expects
// <owner>+<pod-short-name> and resolves it to the owner-scoped DevPod resource
// <owner>-<pod-short-name>. podName is the full resource name returned by K8s.
func (g Gateway) SSHCommand(owner, podName string) string {
	shortName := strings.TrimPrefix(podName, owner+"-")
	login := owner + "+" + shortName
	if g.HostnameSuffix != "" {
		login += "+" + g.HostnameSuffix
	}
	return fmt.Sprintf("ssh %s@%s -p %d", login, g.Host, g.Port)
}
