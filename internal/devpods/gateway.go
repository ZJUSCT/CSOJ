package devpods

import (
	"fmt"

	"github.com/ZJUSCT/CSOJ/internal/config"
)

// Gateway is the stored shape of the `devpods.gateway` setting.
type Gateway struct {
	Host string `json:"host"`
	Port int    `json:"port"`
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

// SSHCommand builds the user-facing login string. podName is the full
// DevPod resource name (e.g. "alice-gpu-8c-a1b2").
func (g Gateway) SSHCommand(username, podName string) string {
	return fmt.Sprintf("ssh %s+%s@%s -p %d", username, podName, g.Host, g.Port)
}
