package command

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	"github.com/julientant/gahelper-plugin/server/gadb"
)

type Handler struct {
	client *pluginapi.Client
}

type Command interface {
	Handle(args *model.CommandArgs) (*model.CommandResponse, error)
	executeHelloCommand(args *model.CommandArgs) *model.CommandResponse
}

const (
	helloCommandTrigger    = "hello"
	gahelperCommandTrigger = "gahelper"

	// CreateFieldDialogURL is the plugin-relative URL the interactive dialog posts back to.
	// Must match the route registered in server/api.go.
	CreateFieldDialogURL = "/plugins/" + gadb.GAHelperPluginID + "/dialog/create-field"
)

// Register all your slash commands in the NewCommandHandler function.
func NewCommandHandler(client *pluginapi.Client) Command {
	err := client.SlashCommand.Register(&model.Command{
		Trigger:          helloCommandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Say hello to someone",
		AutoCompleteHint: "[@username]",
		AutocompleteData: model.NewAutocompleteData(helloCommandTrigger, "[@username]", "Username to say hello to"),
	})
	if err != nil {
		client.Log.Error("Failed to register command", "error", err)
	}

	gahelperAutocomplete := model.NewAutocompleteData(gahelperCommandTrigger, "[create-field|delete-field]", "Dev-only helper for Global Attributes source-column testing (MM-69846)")
	createField := model.NewAutocompleteData("create-field", "", "Opens a dialog to create an access_control/template property field")
	deleteField := model.NewAutocompleteData("delete-field", "[name]", "Soft-deletes an access_control/template property field by Name")
	deleteField.AddTextArgument("The Name of the property field to delete", "[name]", "")
	gahelperAutocomplete.AddCommand(createField)
	gahelperAutocomplete.AddCommand(deleteField)

	err = client.SlashCommand.Register(&model.Command{
		Trigger:          gahelperCommandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Dev-only helper for Global Attributes source-column testing (MM-69846)",
		AutoCompleteHint: "[create-field|delete-field]",
		AutocompleteData: gahelperAutocomplete,
	})
	if err != nil {
		client.Log.Error("Failed to register command", "error", err)
	}

	return &Handler{
		client: client,
	}
}

// ExecuteCommand hook calls this method to execute the commands that were registered in the NewCommandHandler function.
func (c *Handler) Handle(args *model.CommandArgs) (*model.CommandResponse, error) {
	fields := strings.Fields(args.Command)
	if len(fields) == 0 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Empty command",
		}, nil
	}
	trigger := strings.TrimPrefix(fields[0], "/")
	switch trigger {
	case helloCommandTrigger:
		return c.executeHelloCommand(args), nil
	case gahelperCommandTrigger:
		return c.executeGahelperCommand(args, fields[1:]), nil
	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Unknown command: %s", args.Command),
		}, nil
	}
}

func (c *Handler) executeHelloCommand(args *model.CommandArgs) *model.CommandResponse {
	if len(strings.Fields(args.Command)) < 2 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Please specify a username",
		}
	}
	username := strings.Fields(args.Command)[1]
	return &model.CommandResponse{
		Text: "Hello, " + username,
	}
}

func (c *Handler) executeGahelperCommand(args *model.CommandArgs, rest []string) *model.CommandResponse {
	if len(rest) == 0 {
		return ephemeral("Usage: `/gahelper create-field` or `/gahelper delete-field <name>`")
	}

	switch rest[0] {
	case "create-field":
		return c.executeCreateField(args)
	case "delete-field":
		return c.executeDeleteField(rest[1:])
	default:
		return ephemeral(fmt.Sprintf("Unknown /gahelper subcommand: %q. Use create-field or delete-field.", rest[0]))
	}
}

func (c *Handler) executeCreateField(args *model.CommandArgs) *model.CommandResponse {
	dialogRequest := model.OpenDialogRequest{
		TriggerId: args.TriggerId,
		URL:       CreateFieldDialogURL,
		Dialog:    createFieldDialog(),
	}

	if err := c.client.Frontend.OpenInteractiveDialog(dialogRequest); err != nil {
		c.client.Log.Error("gahelper: failed to open create-field dialog", "error", err)
		return ephemeral(fmt.Sprintf("Failed to open dialog: %s", err.Error()))
	}

	return &model.CommandResponse{}
}

func (c *Handler) executeDeleteField(nameArgs []string) *model.CommandResponse {
	name := strings.TrimSpace(strings.Join(nameArgs, " "))
	if name == "" {
		return ephemeral("Usage: `/gahelper delete-field <name>`")
	}

	db, err := c.client.Store.GetMasterDB()
	if err != nil {
		return ephemeral(fmt.Sprintf("Failed to get DB handle: %s", err.Error()))
	}

	found, err := gadb.DeleteFieldByName(db, name)
	if err != nil {
		c.client.Log.Error("gahelper: failed to delete field", "name", name, "error", err)
		return ephemeral(fmt.Sprintf("Failed to delete field %q: %s", name, err.Error()))
	}

	if !found {
		return ephemeral(fmt.Sprintf("No live access_control/template field named %q found.", name))
	}

	return ephemeral(fmt.Sprintf("Deleted field %q. Reload the Manage Attributes page to see the change.", name))
}

func ephemeral(text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         text,
	}
}

// createFieldDialog builds the interactive dialog for `/gahelper create-field`. See
// server/api.go's handleCreateFieldDialog for how the submission is processed.
func createFieldDialog() model.Dialog {
	return model.Dialog{
		CallbackId:  "gahelper-create-field",
		Title:       "GA Helper: Create Property Field (dev-only)",
		SubmitLabel: "Create",
		Elements: []model.DialogElement{
			{
				DisplayName: "Name",
				Name:        "name",
				Type:        "text",
				HelpText:    "PropertyFields.Name — a slug-like identifier, e.g. security_clearance",
			},
			{
				DisplayName: "Display name",
				Name:        "display_name",
				Type:        "text",
				Optional:    true,
				HelpText:    "attrs.display_name shown in the admin UI. Falls back to Name if left blank.",
			},
			{
				DisplayName: "Type",
				Name:        "type",
				Type:        "select",
				Default:     "text",
				Options: []*model.PostActionOptions{
					{Text: "text", Value: "text"},
					{Text: "select", Value: "select"},
					{Text: "multiselect", Value: "multiselect"},
					{Text: "rank", Value: "rank"},
				},
			},
			{
				DisplayName: "Options",
				Name:        "options",
				Type:        "textarea",
				Optional:    true,
				HelpText:    "one option per line — only used for select/multiselect/rank types",
			},
			{
				DisplayName: "Source",
				Name:        "source",
				Type:        "select",
				Default:     "none",
				HelpText:    "Which Source-column branch to demo. Only one of ldap/saml/plugin is ever set at a time.",
				Options: []*model.PostActionOptions{
					{Text: "none (Managed here)", Value: "none"},
					{Text: "ldap (AD/LDAP)", Value: "ldap"},
					{Text: "saml (SAML)", Value: "saml"},
					{Text: "plugin (source_plugin_id + protected)", Value: "plugin"},
				},
			},
			{
				DisplayName: "LDAP attribute",
				Name:        "ldap_attribute",
				Type:        "text",
				Optional:    true,
				HelpText:    "AD/LDAP attribute name this field syncs from. Only used when Source = ldap.",
			},
			{
				DisplayName: "SAML attribute",
				Name:        "saml_attribute",
				Type:        "text",
				Optional:    true,
				HelpText:    "SAML attribute name this field syncs from. Only used when Source = saml.",
			},
		},
	}
}
