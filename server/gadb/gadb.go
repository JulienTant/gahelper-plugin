// Package gadb contains throwaway, local-dev-only helpers for writing directly to the
// PropertyGroups/PropertyFields tables so a human can visually test the Global Attributes
// "Source" column (MM-69846). This is NOT production code and bypasses all app-layer
// validation on purpose. Do not reuse this package outside the gahelper dev plugin.
package gadb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

const (
	// AccessControlGroupName is the PropertyGroups.Name row that owns Global Attributes
	// template fields.
	AccessControlGroupName = "access_control"

	// GAHelperPluginID is stamped into attrs.source_plugin_id when simulating the
	// "plugin" source branch. It does not correspond to a real installed plugin ID other
	// than this dev-only helper itself.
	GAHelperPluginID = "com.mattermost.gahelper"
)

// Option is the {"id": "...", "name": "..."} shape used in attrs.options for
// select/multiselect/rank property fields (see model.PropertyOption in the mattermost repo).
type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateFieldParams describes a PropertyFields row to insert.
type CreateFieldParams struct {
	Name        string
	DisplayName string
	Type        string // text, select, multiselect, date, user, multiuser, rank
	Options     []string

	// Source selects which single Source-column branch to demo: "none", "ldap", "saml", or "plugin".
	Source        string
	LDAPAttribute string
	SAMLAttribute string
}

// typesWithOptions are the property_field_type values that carry attrs.options.
var typesWithOptions = map[string]bool{
	"select":      true,
	"multiselect": true,
	"rank":        true,
}

// EnsureAccessControlGroup returns the real ID of the PropertyGroups row named
// "access_control", creating it first if it doesn't exist yet.
func EnsureAccessControlGroup(db *sql.DB) (string, error) {
	var id string
	err := db.QueryRow(`SELECT ID FROM PropertyGroups WHERE Name = $1`, AccessControlGroupName).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("querying PropertyGroups: %w", err)
	}

	id = model.NewId()
	_, err = db.Exec(`INSERT INTO PropertyGroups (ID, Name) VALUES ($1, $2)`, id, AccessControlGroupName)
	if err != nil {
		return "", fmt.Errorf("inserting PropertyGroups row: %w", err)
	}
	return id, nil
}

// CreateField inserts a new access_control/template PropertyFields row and returns its new ID.
func CreateField(db *sql.DB, params CreateFieldParams) (string, error) {
	groupID, err := EnsureAccessControlGroup(db)
	if err != nil {
		return "", err
	}

	displayName := strings.TrimSpace(params.DisplayName)
	if displayName == "" {
		displayName = params.Name
	}

	attrs := map[string]any{
		"display_name": displayName,
		"visibility":   "when_set",
	}

	if typesWithOptions[params.Type] {
		options := make([]Option, 0, len(params.Options))
		for _, optName := range params.Options {
			optName = strings.TrimSpace(optName)
			if optName == "" {
				continue
			}
			options = append(options, Option{ID: model.NewId(), Name: optName})
		}
		if len(options) > 0 {
			attrs["options"] = options
		}
	}

	protected := false
	switch params.Source {
	case "ldap":
		ldapAttr := strings.TrimSpace(params.LDAPAttribute)
		if ldapAttr == "" {
			ldapAttr = "sAMAccountName"
		}
		attrs["ldap"] = ldapAttr
	case "saml":
		samlAttr := strings.TrimSpace(params.SAMLAttribute)
		if samlAttr == "" {
			samlAttr = "mail"
		}
		attrs["saml"] = samlAttr
	case "plugin":
		attrs["source_plugin_id"] = GAHelperPluginID
		attrs["protected"] = true
		protected = true
	case "", "none":
		// Intentionally set nothing else so the field falls through to "Managed here".
	default:
		return "", fmt.Errorf("unknown source kind %q", params.Source)
	}

	attrsJSON, err := json.Marshal(attrs)
	if err != nil {
		return "", fmt.Errorf("marshalling attrs: %w", err)
	}

	id := model.NewId()
	now := model.GetMillis()

	_, err = db.Exec(`
		INSERT INTO PropertyFields
			(ID, GroupID, Name, Type, Attrs, TargetID, TargetType, CreateAt, UpdateAt, DeleteAt, ObjectType, Protected, PermissionField, PermissionValues, PermissionOptions, LinkedFieldID)
		VALUES
			($1, $2, $3, $4::property_field_type, $5::jsonb, $6, $7, $8, $8, 0, $9, $10, NULL, NULL, NULL, NULL)
	`, id, groupID, params.Name, params.Type, string(attrsJSON), "", "system", now, "template", protected)
	if err != nil {
		return "", fmt.Errorf("inserting PropertyFields row: %w", err)
	}

	return id, nil
}

// DeleteFieldByName soft-deletes (DeleteAt = now) the access_control/template field with the
// given Name. Returns found=false if no live field with that name exists.
func DeleteFieldByName(db *sql.DB, name string) (found bool, err error) {
	var groupID string
	err = db.QueryRow(`SELECT ID FROM PropertyGroups WHERE Name = $1`, AccessControlGroupName).Scan(&groupID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("querying PropertyGroups: %w", err)
	}

	res, err := db.Exec(`
		UPDATE PropertyFields
		SET DeleteAt = $1
		WHERE GroupID = $2 AND Name = $3 AND ObjectType = 'template' AND TargetType = 'system' AND DeleteAt = 0
	`, model.GetMillis(), groupID, name)
	if err != nil {
		return false, fmt.Errorf("soft-deleting PropertyFields row: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}

	return n > 0, nil
}
