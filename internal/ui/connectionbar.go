package ui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/carabila/lazyftp/internal/client"
	"github.com/carabila/lazyftp/internal/config"
)

type connField int

const (
	fieldProtocol connField = iota
	fieldHost
	fieldPort
	fieldUser
	fieldAuth
	fieldPass
	fieldIdentity
	fieldKeyPassphrase
	fieldCount
)

type ConnectionBar struct {
	protocol         client.Protocol
	auth             client.SFTPAuthMethod
	inputs           [fieldCount]textinput.Model
	focused          connField
	picker           keyFilePicker
	profiles         []config.Profile
	profilesLoaded   bool
	profileLoadError string
	profileNotice    string
	selectedProfile  string
	profilePicker    profilePickerState
	profileSave      profileSaveState
}

func NewConnectionBar() ConnectionBar {
	newInput := func(placeholder string, width int) textinput.Model {
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = placeholder
		input.SetWidth(width)
		return input
	}

	host := newInput("Host", 32)
	user := newInput("User", 32)
	pass := newInput("Pass", 32)
	pass.EchoMode = textinput.EchoPassword
	port := newInput("", 8)
	identity := newInput("~/.ssh/id_ed25519", 32)
	keyPassphrase := newInput("Key passphrase", 32)
	keyPassphrase.EchoMode = textinput.EchoPassword

	bar := ConnectionBar{
		inputs: [fieldCount]textinput.Model{
			fieldHost:          host,
			fieldPort:          port,
			fieldUser:          user,
			fieldPass:          pass,
			fieldIdentity:      identity,
			fieldKeyPassphrase: keyPassphrase,
		},
		focused: fieldProtocol,
	}
	return bar.showDefaultPort()
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (c ConnectionBar) showDefaultPort() ConnectionBar {
	c.inputs[fieldPort].Placeholder = strconv.Itoa(c.protocol.DefaultPort())
	return c
}

func (c ConnectionBar) visibleFields() []connField {
	fields := []connField{fieldProtocol, fieldHost, fieldPort, fieldUser}
	if c.protocol != client.SFTP {
		return append(fields, fieldPass)
	}

	fields = append(fields, fieldAuth)
	if c.auth == client.SFTPAuthIdentityFile {
		return append(fields, fieldIdentity, fieldKeyPassphrase)
	}
	return append(fields, fieldPass)
}

func isInputField(field connField) bool {
	switch field {
	case fieldHost, fieldPort, fieldUser, fieldPass, fieldIdentity, fieldKeyPassphrase:
		return true
	default:
		return false
	}
}

func (c ConnectionBar) focus() ConnectionBar {
	if isInputField(c.focused) {
		c.inputs[c.focused].Focus()
	}
	return c
}

func (c ConnectionBar) blur() ConnectionBar {
	if isInputField(c.focused) {
		c.inputs[c.focused].Blur()
	}
	return c
}

func (c ConnectionBar) setFocus(field connField) ConnectionBar {
	c = c.blur()
	c.focused = field
	return c.focus()
}

func (c ConnectionBar) typingText() bool {
	return isInputField(c.focused) && c.focused != fieldPort
}

func (c ConnectionBar) Update(msg tea.Msg) (ConnectionBar, tea.Cmd) {
	switch msg := msg.(type) {
	case profilesLoadedMsg:
		return c.withProfilesLoaded(msg), nil
	case profileWriteDoneMsg:
		return c.withProfileWriteDone(msg), nil
	case keyFilePickerLoadedMsg:
		c.picker = c.picker.withEntries(msg)
		return c, nil
	}

	if c.picker.open {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			return c.updatePicker(keyMsg)
		}
		return c, nil
	}
	if c.profilePicker.open {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			return c.updateProfilePicker(keyMsg)
		}
		return c, nil
	}
	if c.profileSave.open {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			return c.updateProfileSave(keyMsg)
		}
		return c, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keyProfiles):
			return c.openProfiles(), nil
		case key.Matches(msg, keySaveProfile):
			if !c.profilesLoaded {
				c.profileNotice = "Saved profiles are still loading"
				return c, nil
			}
			if c.profileLoadError != "" {
				c.profileNotice = "Cannot save profiles; see the Log"
				return c, nil
			}
			return c.openProfileSave(), nil
		case key.Matches(msg, keyNextField):
			fields := c.visibleFields()
			for i, field := range fields {
				if field == c.focused {
					return c.setFocus(fields[(i+1)%len(fields)]), nil
				}
			}
			return c.setFocus(fields[0]), nil

		case key.Matches(msg, keyPrevField):
			fields := c.visibleFields()
			for i, field := range fields {
				if field == c.focused {
					return c.setFocus(fields[(i+len(fields)-1)%len(fields)]), nil
				}
			}
			return c.setFocus(fields[len(fields)-1]), nil

		case key.Matches(msg, keySubmit):
			return c, func() tea.Msg {
				return ConnectMsg{
					Protocol:      c.protocol,
					Host:          c.inputs[fieldHost].Value(),
					User:          c.inputs[fieldUser].Value(),
					Pass:          c.inputs[fieldPass].Value(),
					Port:          c.inputs[fieldPort].Value(),
					SFTPAuth:      c.auth,
					IdentityFile:  c.inputs[fieldIdentity].Value(),
					KeyPassphrase: c.inputs[fieldKeyPassphrase].Value(),
				}
			}
		}

		if c.focused == fieldProtocol {
			switch {
			case key.Matches(msg, keySelectorPrev):
				c.protocol = c.protocol.Prev()
			case key.Matches(msg, keySelectorNext):
				c.protocol = c.protocol.Next()
			}
			return c.showDefaultPort(), nil
		}

		if c.focused == fieldAuth {
			switch {
			case key.Matches(msg, keySelectorPrev):
				c.auth = client.SFTPAuthPassword
			case key.Matches(msg, keySelectorNext):
				c.auth = client.SFTPAuthIdentityFile
			}
			return c, nil
		}

		if c.focused == fieldIdentity && key.Matches(msg, keyBrowseIdentity) {
			c.picker = newKeyFilePicker()
			return c, c.picker.load()
		}

		// Port only accepts digits. Text is empty for non-printable keys
		// (backspace, arrows, ...), which must still reach the input.
		if c.focused == fieldPort && msg.Text != "" && !isDigits(msg.Text) {
			return c, nil
		}
	}

	if !isInputField(c.focused) {
		return c, nil
	}

	var cmd tea.Cmd
	c.inputs[c.focused], cmd = c.inputs[c.focused].Update(msg)
	return c, cmd
}

func (c ConnectionBar) updatePicker(msg tea.KeyPressMsg) (ConnectionBar, tea.Cmd) {
	switch {
	case key.Matches(msg, keyPickerCancel):
		c.picker.open = false
		return c, nil
	case key.Matches(msg, keyPickerUp):
		c.picker.move(-1)
	case key.Matches(msg, keyPickerDown):
		c.picker.move(1)
	case key.Matches(msg, keyPickerParent):
		var cmd tea.Cmd
		c.picker, cmd = c.picker.parent()
		return c, cmd
	case key.Matches(msg, keyPickerSelect):
		if entry, ok := c.picker.selectedEntry(); ok {
			if entry.isDir {
				var cmd tea.Cmd
				c.picker, cmd = c.picker.openDir(entry.name)
				return c, cmd
			}
			c.inputs[fieldIdentity].SetValue(c.picker.entryPath(entry.name))
			c.picker.open = false
		}
	}
	return c, nil
}

// View renders the connection form as a self-contained floating dialog, no
// wider than maxWidth. It is always shown focused: the app only renders it
// at all while it holds focus.
func (c ConnectionBar) View(maxWidth int) string {
	if c.picker.open {
		return c.picker.View(maxWidth)
	}
	if c.profilePicker.open {
		return c.profilePickerView(maxWidth)
	}
	if c.profileSave.open {
		return c.profileSaveView(maxWidth)
	}

	width := 56
	if width > maxWidth-2 {
		width = maxWidth - 2
	}
	if width < 20 {
		width = 20
	}

	labelStyle := lipgloss.NewStyle().Foreground(colorEmphasis).Bold(true).Width(10)
	arrowStyle := lipgloss.NewStyle().Foreground(colorBorder)

	protocol := c.protocol.String()
	if c.focused == fieldProtocol {
		protocol = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(protocol)
	}
	protocolArrows := arrowStyle.Render("◂ ") + protocol + arrowStyle.Render(" ▸")

	auth := "Password"
	if c.auth == client.SFTPAuthIdentityFile {
		auth = "Identity file"
	}
	if c.focused == fieldAuth {
		auth = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(auth)
	}
	authArrows := arrowStyle.Render("◂ ") + auth + arrowStyle.Render(" ▸")

	row := func(label string, ti textinput.Model) string {
		return labelStyle.Render(label+":") + " " + ti.View()
	}
	fields := []string{
		labelStyle.Render("Protocol:") + " " + protocolArrows,
		row("Host", c.inputs[fieldHost]),
		row("Port", c.inputs[fieldPort]),
		row("User", c.inputs[fieldUser]),
	}
	if c.protocol == client.SFTP {
		fields = append(fields, labelStyle.Render("Auth:")+" "+authArrows)
		if c.auth == client.SFTPAuthIdentityFile {
			fields = append(fields,
				row("Identity", c.inputs[fieldIdentity]),
				row("Key pass", c.inputs[fieldKeyPassphrase]),
			)
		} else {
			fields = append(fields, row("Pass", c.inputs[fieldPass]))
		}
	} else {
		fields = append(fields, row("Pass", c.inputs[fieldPass]))
	}

	hint := "Enter connect · Esc cancel"
	if c.focused == fieldIdentity {
		hint = "Ctrl+O browse · Enter connect · Esc cancel"
	}
	body := strings.Join(fields, "\n\n") + "\n\n" + lipgloss.NewStyle().Foreground(colorMuted).Render(hint)
	if c.profileNotice != "" {
		body += "\n" + lipgloss.NewStyle().Foreground(colorMuted).Render(c.profileNotice)
	}

	height := lipgloss.Height(body) + 2
	return borderWithTitle(body, "Connection", width, height, colorAccent)
}

type ConnectMsg struct {
	Protocol      client.Protocol
	Host          string
	User          string
	Pass          string
	Port          string
	SFTPAuth      client.SFTPAuthMethod
	IdentityFile  string
	KeyPassphrase string
}
