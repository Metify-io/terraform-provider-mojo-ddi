// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

// Command codegen reads a MOJO MCP tool schema JSON file and generates
// boilerplate Go resource files for the Terraform provider.
//
// Usage:
//
//	go run ./tools/codegen -schema docs/mcp-tool-schemas/ipam.json -out internal/resources/ipam/
//	go run ./tools/codegen -schema docs/mcp-tool-schemas/ipam.json -out internal/resources/ipam/ -validate
//
// The generated files follow the same CRUD pattern as vrf_resource.go and
// should be checked into source control after review.
//
// In -validate mode, no files are written. Instead the tool checks whether
// each generated file would differ from the one already on disk and reports
// drift. This is useful for CI to ensure hand-edited resources stay
// consistent with the MCP schemas.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"
	"unicode"
)

// -------------------------------------------------------------------
// MCP tool schema types (subset we care about for code generation)
// -------------------------------------------------------------------

// ToolSchema represents a single MCP tool definition as exported by the server.
type ToolSchema struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]PropertySchema `json:"properties"`
	Required   []string                  `json:"required"`
}

type PropertySchema struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Format      string `json:"format,omitempty"`
}

// -------------------------------------------------------------------
// Codegen types
// -------------------------------------------------------------------

type ResourceSpec struct {
	Domain       string
	ResourceName string
	TFType       string
	CreateTool   string
	ReadTool     string
	UpdateTool   string
	DeleteTool   string
	Fields       []FieldSpec
}

type FieldSpec struct {
	Name        string
	GoType      string
	TFType      string
	Required    bool
	Description string
	Computed    bool
	IsID        bool
}

// -------------------------------------------------------------------
// Main
// -------------------------------------------------------------------

func main() {
	schemaPath := flag.String("schema", "", "Path to MCP tool schema JSON")
	outDir := flag.String("out", ".", "Output directory for generated Go files")
	validate := flag.Bool("validate", false, "Validate existing files match schemas (no write)")
	flag.Parse()

	if *schemaPath == "" {
		fmt.Fprintln(os.Stderr, "usage: codegen -schema <path> [-out <dir>] [-validate]")
		os.Exit(1)
	}

	data, err := os.ReadFile(*schemaPath)
	if err != nil {
		fatalf("read schema: %v", err)
	}

	var tools []ToolSchema
	if err := json.Unmarshal(data, &tools); err != nil {
		fatalf("parse schema: %v", err)
	}

	specs := deriveResources(tools)

	if *validate {
		drifted := 0
		for _, spec := range specs {
			outPath := fmt.Sprintf("%s/%s_resource.go", *outDir, spec.ResourceName)
			generated, err := renderResource(spec)
			if err != nil {
				fatalf("render %s: %v", spec.ResourceName, err)
			}

			existing, readErr := os.ReadFile(outPath)
			if readErr != nil {
				fmt.Printf("MISSING  %s (would be generated from %s)\n", outPath, spec.CreateTool)
				drifted++
				continue
			}

			if string(existing) != generated {
				fmt.Printf("DRIFT    %s (hand-edited file differs from schema)\n", outPath)
				drifted++
			} else {
				fmt.Printf("OK       %s\n", outPath)
			}
		}
		if drifted > 0 {
			fmt.Printf("\n%d file(s) have drift. Run without -validate to regenerate.\n", drifted)
			os.Exit(1)
		}
		fmt.Println("\nAll generated files are consistent with schemas.")
		return
	}

	for _, spec := range specs {
		if err := generateResource(spec, *outDir); err != nil {
			fatalf("generate %s: %v", spec.ResourceName, err)
		}
		fmt.Printf("generated %s/%s_resource.go\n", *outDir, spec.ResourceName)
	}
}

// deriveResources groups tools into CRUD sets and builds ResourceSpec per resource.
func deriveResources(tools []ToolSchema) []ResourceSpec {
	byName := make(map[string]ToolSchema, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
	}

	var specs []ResourceSpec
	for _, t := range tools {
		parts := strings.SplitN(t.Name, ".", 2)
		if len(parts) != 2 {
			continue
		}
		domain, action := parts[0], parts[1]
		if !strings.HasPrefix(action, "create_") {
			continue
		}

		resName := strings.TrimPrefix(action, "create_")
		spec := ResourceSpec{
			Domain:       domain,
			ResourceName: resName,
			TFType:       "mojo_" + resName,
			CreateTool:   t.Name,
			ReadTool:     domain + ".read_" + resName,
			UpdateTool:   domain + ".update_" + resName,
			DeleteTool:   domain + ".delete_" + resName,
		}

		required := make(map[string]bool)
		for _, r := range t.InputSchema.Required {
			required[r] = true
		}

		spec.Fields = append(spec.Fields, FieldSpec{
			Name:     "id",
			GoType:   "types.String",
			TFType:   "StringAttribute",
			Computed: true,
			IsID:     true,
		})

		for propName, prop := range t.InputSchema.Properties {
			if propName == "id" {
				continue
			}
			field := FieldSpec{
				Name:        propName,
				Description: prop.Description,
				Required:    required[propName],
				Computed:    !required[propName],
			}
			switch prop.Type {
			case "boolean":
				field.GoType = "types.Bool"
				field.TFType = "BoolAttribute"
			case "integer":
				field.GoType = "types.Int64"
				field.TFType = "Int64Attribute"
			default:
				field.GoType = "types.String"
				field.TFType = "StringAttribute"
			}
			spec.Fields = append(spec.Fields, field)
		}

		specs = append(specs, spec)
	}

	return specs
}

// -------------------------------------------------------------------
// Template
// -------------------------------------------------------------------

const resourceTemplate = `// Code generated by tools/codegen. DO NOT EDIT.
// Source: {{ .CreateTool }}
// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

package {{ .Domain }}resources

import (
	"context"
	"fmt"

	"github.com/Metify-io/terraform-provider-mojo-ddi/internal/mcp"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &{{ .GoName }}Resource{}
var _ resource.ResourceWithImportState = &{{ .GoName }}Resource{}

func New{{ .GoName }}Resource() resource.Resource { return &{{ .GoName }}Resource{} }

type {{ .GoName }}Resource struct{ client *mcp.Client }

type {{ .GoName }}Model struct {
{{ range .Fields }}	{{ .GoFieldName }} {{ .GoType }} ` + "`tfsdk:\"{{ .Name }}\"`" + `
{{ end }}}

type {{ .LowerName }}APIModel struct {
{{ range .Fields }}	{{ .GoFieldName }} {{ .APIGoType }} ` + "`json:\"{{ .Name }}{{ if not .Required }},omitempty{{ end }}\"`" + `
{{ end }}}

func (r *{{ .GoName }}Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_{{ .ResourceName }}"
}

func (r *{{ .GoName }}Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "{{ escapeQuotes .Description }}",
		Attributes: map[string]schema.Attribute{
{{ range .Fields }}			"{{ .Name }}": schema.{{ .TFType }}{
				{{ if .IsID }}Computed:            true,
				MarkdownDescription: "MOJO-assigned UUID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},{{ else if .Required }}Required:            true,
				MarkdownDescription: "{{ escapeQuotes .Description }}",{{ else }}Optional:            true,
				Computed:            true,
				MarkdownDescription: "{{ escapeQuotes .Description }}",{{ end }}
			},
{{ end }}		},
	}
}

func (r *{{ .GoName }}Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*mcp.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected resource provider data", fmt.Sprintf("Expected *mcp.Client, got %T.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *{{ .GoName }}Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan {{ .GoName }}Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
{{ range .Fields }}{{ if and (not .IsID) .Required }}		"{{ .Name }}": plan.{{ .GoFieldName }}.{{ .ValueMethod }},
{{ end }}{{ end }}	}
{{ range .Fields }}{{ if and (not .IsID) (not .Required) }}	{{ .OptionalArgBlock }}
{{ end }}{{ end }}
	var apiObj {{ .LowerName }}APIModel
	if err := r.client.CallToolJSON(ctx, "{{ .CreateTool }}", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to create {{ .ResourceName }}", err.Error())
		return
	}

	{{ .LowerName }}APIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *{{ .GoName }}Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state {{ .GoName }}Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiObj {{ .LowerName }}APIModel
	err := r.client.CallToolJSON(ctx, "{{ .ReadTool }}", map[string]any{"id": state.ID.ValueString()}, &apiObj)
	if err != nil {
		if mcp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read {{ .ResourceName }}", err.Error())
		return
	}

	{{ .LowerName }}APIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *{{ .GoName }}Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan {{ .GoName }}Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	args := map[string]any{
		"id": plan.ID.ValueString(),
	}
{{ range .Fields }}{{ if and (not .IsID) }}	{{ .OptionalArgBlock }}
{{ end }}{{ end }}
	var apiObj {{ .LowerName }}APIModel
	if err := r.client.CallToolJSON(ctx, "{{ .UpdateTool }}", args, &apiObj); err != nil {
		resp.Diagnostics.AddError("Failed to update {{ .ResourceName }}", err.Error())
		return
	}

	{{ .LowerName }}APIToState(&apiObj, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *{{ .GoName }}Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state {{ .GoName }}Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.CallTool(ctx, "{{ .DeleteTool }}", map[string]any{"id": state.ID.ValueString()})
	if err != nil && !mcp.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete {{ .ResourceName }}", err.Error())
	}
}

func (r *{{ .GoName }}Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var apiObj {{ .LowerName }}APIModel
	err := r.client.CallToolJSON(ctx, "{{ .ReadTool }}", map[string]any{"id": req.ID}, &apiObj)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import {{ .ResourceName }}", err.Error())
		return
	}

	var state {{ .GoName }}Model
	{{ .LowerName }}APIToState(&apiObj, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func {{ .LowerName }}APIToState(api *{{ .LowerName }}APIModel, state *{{ .GoName }}Model) {
{{ range .Fields }}	state.{{ .GoFieldName }} = {{ .StateAssignment }}
{{ end }}}
`

// -------------------------------------------------------------------
// Template rendering
// -------------------------------------------------------------------

type tmplFieldData struct {
	FieldSpec
	GoFieldName      string
	APIGoType        string
	ValueMethod      string
	OptionalArgBlock string
	StateAssignment  string
}

type tmplData struct {
	ResourceSpec
	GoName      string
	LowerName   string
	Description string
	Fields      []tmplFieldData
}

func renderResource(spec ResourceSpec) (string, error) {
	goName := toPascalCase(spec.ResourceName)
	lowerName := toLowerCamel(spec.ResourceName)

	var fields []tmplFieldData
	for _, f := range spec.Fields {
		td := tmplFieldData{
			FieldSpec:   f,
			GoFieldName: toPascalCase(f.Name),
		}

		switch f.GoType {
		case "types.Bool":
			td.APIGoType = "bool"
			td.ValueMethod = "ValueBool()"
			td.StateAssignment = fmt.Sprintf("types.BoolValue(api.%s)", td.GoFieldName)
			td.OptionalArgBlock = fmt.Sprintf(
				`if !plan.%s.IsNull() && !plan.%s.IsUnknown() {
		args["%s"] = plan.%s.ValueBool()
	}`, td.GoFieldName, td.GoFieldName, f.Name, td.GoFieldName)
		case "types.Int64":
			td.APIGoType = "int64"
			td.ValueMethod = "ValueInt64()"
			td.StateAssignment = fmt.Sprintf("types.Int64Value(api.%s)", td.GoFieldName)
			td.OptionalArgBlock = fmt.Sprintf(
				`if !plan.%s.IsNull() && !plan.%s.IsUnknown() {
		args["%s"] = plan.%s.ValueInt64()
	}`, td.GoFieldName, td.GoFieldName, f.Name, td.GoFieldName)
		default:
			td.APIGoType = "string"
			td.ValueMethod = "ValueString()"
			td.StateAssignment = fmt.Sprintf("types.StringValue(api.%s)", td.GoFieldName)
			td.OptionalArgBlock = fmt.Sprintf(
				`if v := plan.%s.ValueString(); v != "" {
		args["%s"] = v
	}`, td.GoFieldName, f.Name)
		}

		fields = append(fields, td)
	}

	data := tmplData{
		ResourceSpec: spec,
		GoName:       goName,
		LowerName:    lowerName,
		Description:  spec.TFType + " resource.",
		Fields:       fields,
	}

	funcMap := template.FuncMap{
		"escapeQuotes": func(s string) string {
			return strings.ReplaceAll(s, `"`, `\"`)
		},
	}

	tmpl, err := template.New("resource").Funcs(funcMap).Parse(resourceTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func generateResource(spec ResourceSpec, outDir string) error {
	content, err := renderResource(spec)
	if err != nil {
		return err
	}

	outPath := fmt.Sprintf("%s/%s_resource.go", outDir, spec.ResourceName)
	return os.WriteFile(outPath, []byte(content), 0644)
}

// -------------------------------------------------------------------
// String helpers
// -------------------------------------------------------------------

func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	var b strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		// Handle common acronyms
		upper := strings.ToUpper(p)
		switch upper {
		case "ID", "IP", "VRF", "VLAN", "DNS", "DHCP", "MAC", "PXE", "CIDR", "TTL":
			b.WriteString(upper)
		default:
			runes := []rune(p)
			runes[0] = unicode.ToUpper(runes[0])
			b.WriteString(string(runes))
		}
	}
	return b.String()
}

func toLowerCamel(s string) string {
	pascal := toPascalCase(s)
	if len(pascal) == 0 {
		return pascal
	}
	runes := []rune(pascal)
	// Lowercase all leading uppercase runes (handles acronyms like VRF → vrf,
	// IPAddress → ipAddress, DHCPScope → dhcpScope).
	for i := 0; i < len(runes); i++ {
		if !unicode.IsUpper(runes[i]) {
			break
		}
		// If this uppercase char is followed by a lowercase char, keep it
		// uppercase (it starts the next word). Exception: the first char.
		if i > 0 && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
			break
		}
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "codegen: "+format+"\n", args...)
	os.Exit(1)
}
