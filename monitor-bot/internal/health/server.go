package health

import (
	"encoding/json"
	"net/http"
)

type ServerStatus struct {
	OK                              bool   `json:"ok"`
	HTTPServer                      bool   `json:"httpServer"`
	AWSSDKConfigured                bool   `json:"awsSdkConfigured"`
	DiscordPublicKeyProvided        bool   `json:"discordPublicKeyProvided"`
	DiscordCommandsRegistered       bool   `json:"discordCommandsRegistered"`
	DiscordCommandRegistrationError string `json:"discordCommandRegistrationError,omitempty"`
	DashboardEnabled                bool   `json:"dashboardEnabled"`
	AlertEnabled                    bool   `json:"alertEnabled"`
	Version                         string `json:"version,omitempty"`
}

func Handler(status func() ServerStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status())
	}
}
