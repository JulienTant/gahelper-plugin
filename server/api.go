package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/julientant/gahelper-plugin/server/gadb"
)

// initRouter initializes the HTTP router for the plugin.
func (p *Plugin) initRouter() *mux.Router {
	router := mux.NewRouter()

	// Middleware to require that the user is logged in. The server sets the
	// Mattermost-User-Id header itself when relaying an interactive dialog submission, so
	// this also covers the /dialog/* routes below.
	router.Use(p.MattermostAuthorizationRequired)

	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.HandleFunc("/hello", p.HelloWorld).Methods(http.MethodGet)

	dialogRouter := router.PathPrefix("/dialog").Subrouter()
	dialogRouter.HandleFunc("/create-field", p.handleCreateFieldDialog).Methods(http.MethodPost)

	return router
}

// ServeHTTP demonstrates a plugin that handles HTTP requests by greeting the world.
// The root URL is currently <siteUrl>/plugins/com.mattermost.gahelper/api/v1/.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-ID")
		if userID == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) HelloWorld(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("Hello, world!")); err != nil {
		p.API.LogError("Failed to write response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleCreateFieldDialog is posted to by the server when a user submits the dialog opened
// by /gahelper create-field (see server/command/command.go's createFieldDialog).
func (p *Plugin) handleCreateFieldDialog(w http.ResponseWriter, r *http.Request) {
	var req model.SubmitDialogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.API.LogError("gahelper: failed to decode dialog submission", "error", err)
		writeDialogResponse(w, &model.SubmitDialogResponse{Error: "Failed to decode submission: " + err.Error()})
		return
	}

	if req.Cancelled {
		writeDialogResponse(w, &model.SubmitDialogResponse{})
		return
	}

	name, _ := req.Submission["name"].(string)
	if name == "" {
		writeDialogResponse(w, &model.SubmitDialogResponse{
			Errors: map[string]string{"name": "Name is required"},
		})
		return
	}

	displayName, _ := req.Submission["display_name"].(string)
	fieldType, _ := req.Submission["type"].(string)
	if fieldType == "" {
		fieldType = "text"
	}
	optionsRaw, _ := req.Submission["options"].(string)
	source, _ := req.Submission["source"].(string)
	if source == "" {
		source = "none"
	}
	ldapAttribute, _ := req.Submission["ldap_attribute"].(string)
	samlAttribute, _ := req.Submission["saml_attribute"].(string)

	params := gadb.CreateFieldParams{
		Name:          name,
		DisplayName:   displayName,
		Type:          fieldType,
		Options:       splitLines(optionsRaw),
		Source:        source,
		LDAPAttribute: ldapAttribute,
		SAMLAttribute: samlAttribute,
	}

	db, err := p.client.Store.GetMasterDB()
	if err != nil {
		p.API.LogError("gahelper: failed to get master DB", "error", err)
		writeDialogResponse(w, &model.SubmitDialogResponse{Error: "Failed to get DB handle: " + err.Error()})
		return
	}

	id, err := gadb.CreateField(db, params)
	if err != nil {
		p.API.LogError("gahelper: failed to create field", "error", err)
		writeDialogResponse(w, &model.SubmitDialogResponse{Error: "Failed to create field: " + err.Error()})
		return
	}

	p.notifyFieldCreated(req.UserId, req.ChannelId, id, name)

	writeDialogResponse(w, &model.SubmitDialogResponse{})
}

// notifyFieldCreated posts an ephemeral confirmation with the new field's ID.
//
// Note: the Manage Attributes admin console page does not currently subscribe to the
// property_field_created websocket event, so publishing it here wouldn't cause a live
// refresh — per the task spec, we skip that and just tell the user to reload instead.
func (p *Plugin) notifyFieldCreated(userID, channelID, fieldID, name string) {
	msg := fmt.Sprintf("gahelper: created property field %q with ID `%s`. Reload the Manage Attributes page to see it.", name, fieldID)
	p.API.SendEphemeralPost(userID, &model.Post{
		UserId:    userID,
		ChannelId: channelID,
		Message:   msg,
	})
}

func writeDialogResponse(w http.ResponseWriter, resp *model.SubmitDialogResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			line := s[start:i]
			// Trim a trailing \r for CRLF textareas.
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			out = append(out, line)
			start = i + 1
		}
	}
	return out
}
