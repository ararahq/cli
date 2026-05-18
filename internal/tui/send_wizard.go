package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/tui/components"
)

const (
	wizardStepChooseTemplate = 0
	wizardStepEnterPhone     = 1
	wizardStepFillVariables  = 2
	wizardStepConfirmation   = 3
	wizardStepSending        = 4
	wizardStepResult         = 5

	freeformOptionTitle       = "Send freeform message"
	freeformOptionDescription = "Type a custom message body"
	phoneInputPlaceholder     = "+5511999999999"
	variableInputPlaceholder  = "Enter value"
	brazilCountryCode         = "+55"
	brazilPhoneDigits         = 11
	minPhoneDigitsForPrefix   = 10

	wizardListHeight      = 14
	wizardListWidth       = 60
	templateVariableRegex = `\{\{(\d+)\}\}`

	colorBrandOrange  = "#FF6B35"
	colorSuccessGreen = "#00C853"
	colorErrorRed     = "#FF1744"
	colorDimGray      = "#888888"
	colorWhite        = "#FFFFFF"
)

var (
	wizardTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorBrandOrange))
	wizardLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite))
	wizardDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDimGray))
	wizardSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccessGreen))
	wizardErrorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorErrorRed))

	variablePattern = regexp.MustCompile(templateVariableRegex)
)

type templateItem struct {
	name     string
	category string
	body     string
}

func (item templateItem) Title() string       { return item.name }
func (item templateItem) Description() string { return item.category }
func (item templateItem) FilterValue() string { return item.name }

type templatesFetchedMsg struct {
	templates  []api.Template
	fetchError error
}

type messageSentMsg struct {
	response  *api.MessageResponse
	sendError error
}

type SendWizardModel struct {
	client           *api.Client
	currentStep      int
	templates        []api.Template
	templateList     list.Model
	phoneInput       textinput.Model
	variableInputs   []textinput.Model
	freeformInput    textinput.Model
	sendSpinner      spinner.Model
	selectedTemplate *api.Template
	isFreeform       bool
	variableNames    []string
	sentResponse     *api.MessageResponse
	errorMessage     string
	quitting         bool
	windowWidth      int
	windowHeight     int
}

func NewSendWizardModel(client *api.Client) SendWizardModel {
	phoneInput := textinput.New()
	phoneInput.Placeholder = phoneInputPlaceholder
	phoneInput.CharLimit = 20

	freeformInput := textinput.New()
	freeformInput.Placeholder = "Type your message..."
	freeformInput.CharLimit = 1024

	sendSpinner := spinner.New()
	sendSpinner.Spinner = spinner.Dot

	templateList := list.New([]list.Item{}, list.NewDefaultDelegate(), wizardListWidth, wizardListHeight)
	templateList.Title = "Choose a template"
	templateList.SetShowStatusBar(false)
	templateList.SetShowHelp(true)

	return SendWizardModel{
		client:        client,
		currentStep:   wizardStepChooseTemplate,
		templateList:  templateList,
		phoneInput:    phoneInput,
		freeformInput: freeformInput,
		sendSpinner:   sendSpinner,
	}
}

func (model SendWizardModel) Init() tea.Cmd {
	return tea.Batch(fetchTemplatesCmd(model.client), model.sendSpinner.Tick)
}

func (model SendWizardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typedMessage := message.(type) {
	case tea.KeyMsg:
		return model.handleKeyPress(typedMessage)
	case tea.WindowSizeMsg:
		model.windowWidth = typedMessage.Width
		model.windowHeight = typedMessage.Height
		model.templateList.SetSize(typedMessage.Width, typedMessage.Height-4)
		return model, nil
	case templatesFetchedMsg:
		return model.handleTemplatesFetched(typedMessage), nil
	case messageSentMsg:
		return model.handleMessageSent(typedMessage), nil
	case spinner.TickMsg:
		var spinnerCmd tea.Cmd
		model.sendSpinner, spinnerCmd = model.sendSpinner.Update(message)
		return model, spinnerCmd
	}

	return model.updateCurrentInput(message)
}

func (model SendWizardModel) View() string {
	if model.quitting {
		return ""
	}

	header := wizardTitleStyle.Render("arara send wizard")
	separator := wizardDimStyle.Render("───────────────────────────────")

	var body string

	switch model.currentStep {
	case wizardStepChooseTemplate:
		body = model.viewChooseTemplate()
	case wizardStepEnterPhone:
		body = model.viewEnterPhone()
	case wizardStepFillVariables:
		body = model.viewFillVariables()
	case wizardStepConfirmation:
		body = model.viewConfirmation()
	case wizardStepSending:
		body = model.viewSending()
	case wizardStepResult:
		body = model.viewResult()
	}

	return fmt.Sprintf("\n%s\n%s\n\n%s\n", header, separator, body)
}

func (model SendWizardModel) handleKeyPress(keyMessage tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMessage.String() {
	case "ctrl+c", "q":
		if model.currentStep == wizardStepChooseTemplate {
			model.quitting = true
			return model, tea.Quit
		}
	case "esc":
		if model.currentStep > wizardStepChooseTemplate && model.currentStep < wizardStepSending {
			model.currentStep--
			return model, nil
		}
		model.quitting = true
		return model, tea.Quit
	case "enter":
		return model.handleEnterKey()
	}

	return model.updateCurrentInput(tea.Msg(keyMessage))
}

func (model SendWizardModel) handleEnterKey() (tea.Model, tea.Cmd) {
	switch model.currentStep {
	case wizardStepChooseTemplate:
		return model.selectTemplate()
	case wizardStepEnterPhone:
		return model.submitPhone()
	case wizardStepFillVariables:
		return model.submitVariables()
	case wizardStepConfirmation:
		return model.confirmAndSend()
	case wizardStepResult:
		model.quitting = true
		return model, tea.Quit
	}

	return model, nil
}

func (model SendWizardModel) updateCurrentInput(message tea.Msg) (tea.Model, tea.Cmd) {
	var updateCmd tea.Cmd

	switch model.currentStep {
	case wizardStepChooseTemplate:
		model.templateList, updateCmd = model.templateList.Update(message)
	case wizardStepEnterPhone:
		if model.isFreeform {
			model.freeformInput, updateCmd = model.freeformInput.Update(message)
			if !model.freeformInput.Focused() {
				model.phoneInput, updateCmd = model.phoneInput.Update(message)
			}
		} else {
			model.phoneInput, updateCmd = model.phoneInput.Update(message)
		}
	case wizardStepFillVariables:
		for inputIndex := range model.variableInputs {
			if model.variableInputs[inputIndex].Focused() {
				model.variableInputs[inputIndex], updateCmd = model.variableInputs[inputIndex].Update(message)
				break
			}
		}
	}

	return model, updateCmd
}

func (model SendWizardModel) handleTemplatesFetched(fetchMessage templatesFetchedMsg) SendWizardModel {
	if fetchMessage.fetchError != nil {
		model.errorMessage = fmt.Sprintf("Failed to load templates: %s", fetchMessage.fetchError.Error())
		return model
	}

	model.templates = fetchMessage.templates

	items := make([]list.Item, 0, len(fetchMessage.templates)+1)
	items = append(items, templateItem{
		name:     freeformOptionTitle,
		category: freeformOptionDescription,
	})

	for _, template := range fetchMessage.templates {
		items = append(items, templateItem{
			name:     template.Name,
			category: template.Category,
			body:     template.Body,
		})
	}

	model.templateList.SetItems(items)

	return model
}

func (model SendWizardModel) handleMessageSent(sentMessage messageSentMsg) SendWizardModel {
	model.currentStep = wizardStepResult

	if sentMessage.sendError != nil {
		model.errorMessage = fmt.Sprintf("Failed to send message: %s", sentMessage.sendError.Error())
		return model
	}

	model.sentResponse = sentMessage.response
	model.errorMessage = ""

	return model
}

func (model SendWizardModel) selectTemplate() (tea.Model, tea.Cmd) {
	selectedItem, isValid := model.templateList.SelectedItem().(templateItem)
	if !isValid {
		return model, nil
	}

	if selectedItem.name == freeformOptionTitle {
		model.isFreeform = true
		model.selectedTemplate = nil
	} else {
		model.isFreeform = false
		for templateIndex := range model.templates {
			if model.templates[templateIndex].Name == selectedItem.name {
				model.selectedTemplate = &model.templates[templateIndex]
				break
			}
		}
	}

	model.currentStep = wizardStepEnterPhone
	model.phoneInput.Focus()

	return model, textinput.Blink
}

func (model SendWizardModel) submitPhone() (tea.Model, tea.Cmd) {
	phoneValue := strings.TrimSpace(model.phoneInput.Value())

	if phoneValue == "" {
		model.errorMessage = "Phone number is required"
		return model, nil
	}

	normalizedPhone := normalizePhoneForWizard(phoneValue)
	model.phoneInput.SetValue(normalizedPhone)
	model.errorMessage = ""

	if model.isFreeform {
		if strings.TrimSpace(model.freeformInput.Value()) == "" {
			model.errorMessage = "Message body is required for freeform messages"
			model.freeformInput.Focus()
			return model, textinput.Blink
		}
		model.currentStep = wizardStepConfirmation
		return model, nil
	}

	variableNames := extractTemplateVariables(model.selectedTemplate.Body)
	if len(variableNames) == 0 {
		model.currentStep = wizardStepConfirmation
		return model, nil
	}

	model.variableNames = variableNames
	model.variableInputs = make([]textinput.Model, len(variableNames))
	for variableIndex, variableName := range variableNames {
		input := textinput.New()
		input.Placeholder = variableInputPlaceholder
		input.Prompt = fmt.Sprintf("  {{%s}}: ", variableName)
		input.CharLimit = 256
		if variableIndex == 0 {
			input.Focus()
		}
		model.variableInputs[variableIndex] = input
	}

	model.currentStep = wizardStepFillVariables

	return model, textinput.Blink
}

func (model SendWizardModel) submitVariables() (tea.Model, tea.Cmd) {
	focusedIndex := -1
	for inputIndex := range model.variableInputs {
		if model.variableInputs[inputIndex].Focused() {
			focusedIndex = inputIndex
			break
		}
	}

	if focusedIndex >= 0 && focusedIndex < len(model.variableInputs)-1 {
		model.variableInputs[focusedIndex].Blur()
		model.variableInputs[focusedIndex+1].Focus()
		return model, textinput.Blink
	}

	for inputIndex, input := range model.variableInputs {
		if strings.TrimSpace(input.Value()) == "" {
			model.errorMessage = fmt.Sprintf("Variable {{%s}} is required", model.variableNames[inputIndex])
			return model, nil
		}
	}

	model.errorMessage = ""
	model.currentStep = wizardStepConfirmation

	return model, nil
}

func (model SendWizardModel) confirmAndSend() (tea.Model, tea.Cmd) {
	model.currentStep = wizardStepSending
	model.errorMessage = ""

	request := model.buildSendRequest()

	return model, sendMessageCmd(model.client, request)
}

func (model SendWizardModel) buildSendRequest() api.SendMessageRequest {
	request := api.SendMessageRequest{
		Receiver: model.phoneInput.Value(),
	}

	if model.isFreeform {
		request.Body = model.freeformInput.Value()
		return request
	}

	request.TemplateName = model.selectedTemplate.Name

	if len(model.variableInputs) > 0 {
		variables := make([]string, len(model.variableInputs))
		for inputIndex, input := range model.variableInputs {
			variables[inputIndex] = strings.TrimSpace(input.Value())
		}
		request.TemplateVariables = variables
	}

	return request
}

func (model SendWizardModel) viewChooseTemplate() string {
	if model.errorMessage != "" {
		return wizardErrorStyle.Render(model.errorMessage)
	}

	return model.templateList.View()
}

func (model SendWizardModel) viewEnterPhone() string {
	var builder strings.Builder

	builder.WriteString(wizardLabelStyle.Render("Step 2: Enter recipient phone number"))
	builder.WriteString("\n\n")
	builder.WriteString(model.phoneInput.View())
	builder.WriteString("\n")

	if model.isFreeform {
		builder.WriteString("\n")
		builder.WriteString(wizardLabelStyle.Render("Message body:"))
		builder.WriteString("\n")
		builder.WriteString(model.freeformInput.View())
		builder.WriteString("\n")
	}

	if model.errorMessage != "" {
		builder.WriteString("\n")
		builder.WriteString(wizardErrorStyle.Render(model.errorMessage))
	}

	builder.WriteString("\n")
	builder.WriteString(wizardDimStyle.Render("Press Enter to continue, Esc to go back"))

	return builder.String()
}

func (model SendWizardModel) viewFillVariables() string {
	var builder strings.Builder

	builder.WriteString(wizardLabelStyle.Render("Step 3: Fill template variables"))
	builder.WriteString("\n\n")

	for _, input := range model.variableInputs {
		builder.WriteString(input.View())
		builder.WriteString("\n")
	}

	if model.errorMessage != "" {
		builder.WriteString("\n")
		builder.WriteString(wizardErrorStyle.Render(model.errorMessage))
	}

	builder.WriteString("\n")
	builder.WriteString(wizardDimStyle.Render("Press Enter to advance fields, Esc to go back"))

	return builder.String()
}

func (model SendWizardModel) viewConfirmation() string {
	var builder strings.Builder

	builder.WriteString(wizardLabelStyle.Render("Step 4: Confirm and send"))
	builder.WriteString("\n\n")

	fmt.Fprintf(&builder, "  %-12s %s\n", "To:", model.phoneInput.Value())

	if model.isFreeform {
		fmt.Fprintf(&builder, "  %-12s %s\n", "Type:", "Freeform")
		fmt.Fprintf(&builder, "  %-12s %s\n", "Body:", model.freeformInput.Value())
	} else {
		fmt.Fprintf(&builder, "  %-12s %s\n", "Template:", model.selectedTemplate.Name)
		if len(model.variableInputs) > 0 {
			for inputIndex, input := range model.variableInputs {
				label := fmt.Sprintf("{{%s}}:", model.variableNames[inputIndex])
				fmt.Fprintf(&builder, "  %-12s %s\n", label, input.Value())
			}
		}
	}

	builder.WriteString("\n")
	builder.WriteString(components.RenderStatusBadge("PENDING"))
	builder.WriteString("\n\n")
	builder.WriteString(wizardDimStyle.Render("Press Enter to send, Esc to go back"))

	return builder.String()
}

func (model SendWizardModel) viewSending() string {
	return fmt.Sprintf("%s Sending message...", model.sendSpinner.View())
}

func (model SendWizardModel) viewResult() string {
	if model.errorMessage != "" {
		return fmt.Sprintf("%s\n\n%s",
			wizardErrorStyle.Render(model.errorMessage),
			wizardDimStyle.Render("Press Enter to exit"),
		)
	}

	var builder strings.Builder

	builder.WriteString(wizardSuccessStyle.Render("Message sent successfully!"))
	builder.WriteString("\n\n")
	fmt.Fprintf(&builder, "  %-12s %s\n", "ID:", model.sentResponse.ID)
	fmt.Fprintf(&builder, "  %-12s %s\n", "Status:", components.RenderStatusBadge(model.sentResponse.Status))
	fmt.Fprintf(&builder, "  %-12s %s\n", "To:", model.sentResponse.Receiver)
	builder.WriteString("\n")
	builder.WriteString(wizardDimStyle.Render("Press Enter to exit"))

	return builder.String()
}

func fetchTemplatesCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		templates, fetchError := client.ListTemplates()
		return templatesFetchedMsg{templates: templates, fetchError: fetchError}
	}
}

func sendMessageCmd(client *api.Client, request api.SendMessageRequest) tea.Cmd {
	return func() tea.Msg {
		response, sendError := client.SendMessage(request)
		return messageSentMsg{response: response, sendError: sendError}
	}
}

func normalizePhoneForWizard(phone string) string {
	cleaned := strings.ReplaceAll(phone, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")

	if strings.HasPrefix(cleaned, "+") {
		return cleaned
	}

	digitsOnly := strings.TrimLeft(cleaned, "0")

	if len(digitsOnly) >= minPhoneDigitsForPrefix && len(digitsOnly) <= brazilPhoneDigits {
		return brazilCountryCode + digitsOnly
	}

	return "+" + cleaned
}

func extractTemplateVariables(templateBody string) []string {
	matches := variablePattern.FindAllStringSubmatch(templateBody, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	variables := make([]string, 0, len(matches))

	for _, match := range matches {
		variableName := match[1]
		if seen[variableName] {
			continue
		}
		seen[variableName] = true
		variables = append(variables, variableName)
	}

	return variables
}
