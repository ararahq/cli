package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
)

const (
	apiHeaderSeparator = ":"
	apiBodyStdinMarker = "-"
)

var (
	apiBodyFlag   string
	apiHeaderFlag []string
	apiMethodFlag string
	apiInputFlag  string
	apiSilentFlag bool
)

var apiCmd = &cobra.Command{
	Use:   "api <path>",
	Short: "Make an authenticated request to any AraraHQ API endpoint",
	Long: "Make an authenticated request to an arbitrary AraraHQ API endpoint.\n" +
		"Reuses the same auth, retry, idempotency-key, and verbose-redaction logic\n" +
		"as the typed commands. Useful when you need an endpoint we haven't wrapped\n" +
		"in a dedicated subcommand yet.\n\n" +
		"The path is relative to the API base URL of the active profile.\n\n" +
		"Examples:\n" +
		"  arara api /v1/wallet/balance\n" +
		"  arara api /v1/messages/msg_xyz\n" +
		"  arara api -X POST /v1/segments --body '{\"name\":\"VIPs\"}'\n" +
		"  arara api -X DELETE /v1/contacts/contact_123\n" +
		"  arara api /v1/templates --jq '.[] | select(.status==\"APPROVED\") | .name'\n" +
		"  echo '{\"name\":\"Promo\"}' | arara api -X POST /v1/segments --input -",
	Args: cobra.ExactArgs(1),
	RunE: runAPI,
}

func init() {
	apiCmd.Flags().StringVarP(&apiMethodFlag, "method", "X", http.MethodGet, "HTTP method (GET, POST, PUT, PATCH, DELETE)")
	apiCmd.Flags().StringVar(&apiBodyFlag, "body", "", "request body as inline JSON string")
	apiCmd.Flags().StringVar(&apiInputFlag, "input", "", "read request body from file path, or '-' for stdin")
	apiCmd.Flags().StringSliceVarP(&apiHeaderFlag, "header", "H", nil, "extra request header (Name:Value), can be repeated")
	apiCmd.Flags().BoolVar(&apiSilentFlag, "silent", false, "suppress response body output (still respects exit code)")

	rootCmd.AddCommand(apiCmd)
}

func runAPI(_ *cobra.Command, arguments []string) error {
	requestPath := arguments[0]

	if validationError := validateAPIRequest(); validationError != nil {
		output.PrintError(validationError.Error())
		return validationError
	}

	body, bodyError := resolveAPIBody()
	if bodyError != nil {
		output.PrintError(bodyError.Error())
		return bodyError
	}

	headers, headerError := parseAPIHeaders(apiHeaderFlag)
	if headerError != nil {
		output.PrintError(headerError.Error())
		return headerError
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())

	method := strings.ToUpper(strings.TrimSpace(apiMethodFlag))
	return runAPIImpl(client, GetOutputFormat(), os.Stdout, method, requestPath, body, headers, apiSilentFlag)
}

// runAPIImpl is the testable core of `arara api`. Owns the request +
// response rendering. Callers (CLI wrapper, tests) supply the client,
// writer, and validated/parsed inputs.
func runAPIImpl(client *api.Client, format output.Format, writer io.Writer, method string, requestPath string, body any, headers map[string]string, silent bool) error {
	requestPath = ensureLeadingSlash(requestPath)

	rawResponse, requestError := executeAPIRequest(client, method, requestPath, body, headers)
	if requestError != nil {
		return fmt.Errorf("request failed: %w", requestError)
	}

	if silent {
		return nil
	}

	return renderAPIResponseWithFormat(writer, rawResponse, format)
}

func renderAPIResponseWithFormat(writer io.Writer, raw any, format output.Format) error {
	if raw == nil {
		return nil
	}

	filtered, filterError := applyJQIfRequested(raw)
	if filterError != nil {
		return filterError
	}

	switch format {
	case output.FormatStreamJSON:
		return writeJSONLinesTo(writer, filtered)
	default:
		return writeJSONIndentedAllTo(writer, filtered)
	}
}

func validateAPIRequest() error {
	if apiBodyFlag != "" && apiInputFlag != "" {
		return fmt.Errorf("--body and --input are mutually exclusive")
	}
	method := strings.ToUpper(strings.TrimSpace(apiMethodFlag))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return nil
	default:
		return fmt.Errorf("unsupported HTTP method %q (use GET, POST, PUT, PATCH, or DELETE)", apiMethodFlag)
	}
}

func resolveAPIBody() (any, error) {
	if apiBodyFlag != "" {
		return decodeJSONBody([]byte(apiBodyFlag))
	}
	if apiInputFlag == "" {
		return nil, nil
	}

	bytes, readError := readBodyInput(apiInputFlag)
	if readError != nil {
		return nil, readError
	}
	return decodeJSONBody(bytes)
}

func readBodyInput(source string) ([]byte, error) {
	if source == apiBodyStdinMarker {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(source)
}

func decodeJSONBody(raw []byte) (any, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil
	}
	var decoded any
	if unmarshalError := json.Unmarshal([]byte(trimmed), &decoded); unmarshalError != nil {
		return nil, fmt.Errorf("body is not valid JSON: %w", unmarshalError)
	}
	return decoded, nil
}

func parseAPIHeaders(headers []string) (map[string]string, error) {
	if len(headers) == 0 {
		return nil, nil
	}

	result := make(map[string]string, len(headers))
	for _, header := range headers {
		colonIndex := strings.Index(header, apiHeaderSeparator)
		if colonIndex <= 0 || colonIndex == len(header)-1 {
			return nil, fmt.Errorf("invalid header %q — expected Name:Value", header)
		}
		name := strings.TrimSpace(header[:colonIndex])
		value := strings.TrimSpace(header[colonIndex+1:])
		if name == "" || value == "" {
			return nil, fmt.Errorf("invalid header %q — name and value required", header)
		}
		result[name] = value
	}
	return result, nil
}

func executeAPIRequest(client *api.Client, method string, path string, body any, headers map[string]string) (any, error) {
	var raw any
	if requestError := client.DoWithHeaders(method, path, body, &raw, headers); requestError != nil {
		return nil, requestError
	}
	return raw, nil
}

func renderAPIResponse(writer io.Writer, raw any) error {
	if raw == nil {
		return nil
	}

	filtered, filterError := applyJQIfRequested(raw)
	if filterError != nil {
		return filterError
	}

	switch GetOutputFormat() {
	case output.FormatStreamJSON:
		return writeJSONLinesTo(writer, filtered)
	default:
		return writeJSONIndentedAllTo(writer, filtered)
	}
}

// applyJQIfRequested runs the global --jq expression against `raw` when set,
// otherwise returns the input wrapped as a single-element slice. Centralized
// here so every command that emits JSON inherits jq support for free.
func applyJQIfRequested(raw any) ([]any, error) {
	expression := GetJQ()
	if expression == "" {
		return []any{raw}, nil
	}
	return output.ApplyJQ(raw, expression)
}

func writeJSONIndentedAllTo(writer io.Writer, values []any) error {
	for _, value := range values {
		if err := writeJSONIndentedTo(writer, value); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONLinesTo(writer io.Writer, values []any) error {
	for _, value := range values {
		if err := writeJSONLineTo(writer, value); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONIndentedTo(writer io.Writer, raw any) error {
	encoded, encodeError := json.MarshalIndent(raw, "", "  ")
	if encodeError != nil {
		return fmt.Errorf("encode response: %w", encodeError)
	}
	if _, writeError := fmt.Fprintln(writer, string(encoded)); writeError != nil {
		return writeError
	}
	return nil
}

func writeJSONLineTo(writer io.Writer, raw any) error {
	return output.WriteJSONLine(writer, raw)
}

func ensureLeadingSlash(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}
