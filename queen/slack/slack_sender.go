package slack

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	common "plexobject.com/formicary/internal/types"
	"plexobject.com/formicary/queen/config"
	"plexobject.com/formicary/queen/manager"
	"plexobject.com/formicary/queen/types"
	"strings"
)

// DefaultSlackSender defines operations to send slack
type DefaultSlackSender struct {
	cfg         *config.ServerConfig
	userManager *manager.UserManager
}

// New constructor
func New(
	cfg *config.ServerConfig,
	userManager *manager.UserManager,
) (types.Sender, error) {
	return &DefaultSlackSender{
		cfg:         cfg,
		userManager: userManager,
	}, nil
}

// SupportsLongReport is not supported
func (d *DefaultSlackSender) SupportsLongReport() bool {
	return false
}

// SendMessage sends slack to recipients
func (d *DefaultSlackSender) SendMessage(
	qc *common.QueryContext,
	user *common.User,
	to []string,
	subject string,
	body string,
	opts map[string]interface{}) (err error) {
	var token string
	if user.HasOrganization() {
		token = user.Organization.GetConfigString(types.SlackToken)
	}
	if token == "" {
		return fmt.Errorf("SlackToken is not found in organization config")
	}

	api := slack.New(strings.TrimSpace(token))
	attachment := slack.Attachment{
		CallbackID: "view_job",
		//Pretext: subject,
		//Fallback: subject,
		//Title:      subject,
		Text:       body,
		MarkdownIn: []string{"text"},
	}
	if opts[types.Color] != nil {
		attachment.Color = opts[types.Color].(string)
	}
	if opts[types.Link] != nil {
		attachment.TitleLink = opts[types.Link].(string)
		attachment.Footer = fmt.Sprintf("You can find more details at %s", attachment.TitleLink)
		attachment.Actions = []slack.AttachmentAction{{
			Name:  "View",
			Text:  "View",
			Style: "primary",
			Type:  "button",
			URL:   attachment.TitleLink,
		},
		}
	}
	msgOpts := []slack.MsgOption{
		slack.MsgOptionText(subject, false),
		slack.MsgOptionAttachments(attachment),
		slack.MsgOptionAsUser(true),
	}
	if opts[types.Emoji] != nil {
		msgOpts = append(msgOpts, slack.MsgOptionIconEmoji(opts[types.Emoji].(string)))
	}
	if opts[types.Thread] != nil {
		msgOpts = append(msgOpts, slack.MsgOptionTS(opts[types.Thread].(string)))
	}
	if opts[types.LongReport] != nil {
		msgOpts = append(msgOpts, slack.MsgOptionText(opts[types.LongReport].(string), true))
	}
	for _, recipient := range to {
		_, _, err = api.PostMessage(
			recipient,
			msgOpts...,
		)
		if err != nil {
			_ = d.userManager.AddStickyMessageForSlack(
				qc,
				user,
				err)
			return fmt.Errorf("failed to send message to slack due to %w", err)
		}
	}
	logrus.WithFields(logrus.Fields{
		"Component":             "DefaultSlackSender",
		"Subject":               subject,
		"Channel":               to,
		"JobNotifyTemplateFile": d.JobNotifyTemplateFile(),
		"Size":                  len(body),
	}).Infof("sending slack message")

	_ = d.userManager.ClearStickyMessageForSlack(
		qc,
		user,
	)
	return nil
}

// JobNotifyTemplateFile template file
func (d *DefaultSlackSender) JobNotifyTemplateFile() string {
	return d.cfg.Notify.SlackJobsTemplateFile
}
