// Copyright (c) Metify, Inc.
// SPDX-License-Identifier: Apache-2.0

// Command codegen reads a MOJO MCP tool schema JSON file and generates
// boilerplate Go resource files for the Terraform provider.
//
// Usage:
//
//	go run ./tools/codegen -schema docs/mcp-tool-schemas/ipam.json -out internal/resources/ipam/
//
// The generated files follow the same CRUD pattern as vrf_resource.go and
// should be checked into source control after review.
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
	Name        string          `json:"name"`        // e.g. "ipam.create_vrf"
	Description string          `json:"description"` // human-readable
	InputSchema InputSchema     `json:"inputSchema"`
}

type InputSchema struct {
	Type       string                       `json:"type"`
	Properties map[string]PropertySchema    `json:"properties"`
	Required   []string                     `json:"required"`
}

type PropertySchema struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Format      string `json:"format,omitempty"` // "uuid", "cidr", etc.
}

// -------------------------------------------------------------------
// Codegen types
// -------------------------------------------------------------------

// ResourceSpec is what we derive from a set of CRUD tool schemas.
type ResourceSpec struct {
	Domain       string // "ipam"
	ResourceName string // "vrf"
	TFType       string // "mojo_vrf"
	CreateTool   string // "ipam.create_vrf"
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
	flag.Parse()

	if *schemaPath == "" {
		fmt.Fprintln(os.Stderr, "usage: codegen -schema <path> [-out <dir>]")
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
	for _, spec := range specs {
		if err := generateResource(spec, *outDir); err != nil {
			fatalf("generate %s: %v", spec.ResourceName, err)
		}
		fmt.Printf("generated %s/%s_resource.go\n", *outDir, spec.ResourceName)
	}
}

// deriveResources groups tools into CRUD sets and builds ResourceSpec per resource.
func deriveResources(tools []ToolSchema) []ResourceSpec {
	// Index tools by name for lookup
	byName := make(map[string]ToolSchema, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
	}

	// Find all create_* tools — each one defines a resource
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

		// Build fields from the create tool's input schema
		required := make(map[string]bool)
		for _, r := range t.InputSchema.Required {
			required[r] = true
		}

		// Always add an id field (server-assigned, computed)
		spec.Fields = append(spec.Fields, FieldSpec{
			Name:     "id",
			GoType:   "types.String",
			TFType:   "StringAttribute",
			Computed: true,
			IsID:     true,
		})

		for propName, prop := range t.InputSchema.Properties {
			if propName == "id" {
				continue // already added
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

type {{ .GoName }}APIModel struct {
{{ range .Fields }}	{{ .GoFieldName }} {{ .APIGoType }} ` + "`json:\"{{ .Name }},omitempty\"`" + `
{{ end }}}

func (r *{{ .GoName }}Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_{{ .ResourceName }}"
}

func (r *{{ .GoName }}Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
{{ range .Fields }}			"{{ .Name }}": schema.{{ .TFType }}{
				{{ if .IsID }}Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},{{ else if .Required }}Required: true,{{ else }}Optional: true, Computed: true,{{ end }}
				MarkdownDescription: "{{ escapeQuotes .Description }}",
			},
{{ end }}		},
	}
}

func (r *{{ .GoName }}Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil { return }
	client, ok := req.ProviderData.(*mcp.Client)
	if !ok { resp.Diagnostics.AddError("unexpected provider data", fmt.Sprintf("got %T", req.ProviderData)); return }
	r.client = client
}

func (r *{{ .GoName }}Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan {{ .GoName }}Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() { return }
	// TODO: build args from plan, call {{ .CreateTool }}
	_ = plan
	resp.Diagnostics.AddError("not implemented", "Create for {{ .TFType }} is not yet implemented")
}

func (r *{{ .GoName }}Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state {{ .GoName }}Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	// TODO: call {{ .ReadTool }}
	_ = state
}

func (r *{{ .GoName }}Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// TODO: call {{ .UpdateTool }}
}

func (r *{{ .GoName }}Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state {{ .GoName }}Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() { return }
	// TODO: call {{ .DeleteTool }}
	_ = state
}

func (r *{{ .GoName }}Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// TODO: call {{ .ReadTool }} by req.ID
}
`

func generateResource(spec ResourceSpec, outDir string) error {
	type tmplFieldData struct {
		FieldSpec
		GoFieldName string
		APIGoType   string
	}

	type tmplData struct {
		ResourceSpec
		GoName string
		Fields []tmplFieldData
	}

	goName := toPascalCase(spec.ResourceName)

	var fields []tmplFieldData
	for _, f := range spec.Fields {
		apiType := "string"
		switch f.GoType {
		case "types.Bool":
			apiType = "bool"
		case "types.Int64":
			apiType = "int64"
		}
		fields = append(fields, tmplFieldData{
			FieldSpec:   f,
			GoFieldName: toPascalCase(f.Name),
			APIGoType:   apiType,
		})
	}

	data := tmplData{
		ResourceSpec: spec,
		GoName:       goName,
		Fields:       fields,
	}

	funcMap := template.FuncMap{
		"escapeQuotes": func(s string) string {
			return strings.ReplaceAll(s, `"`, `\"`)
		},
	}

	tmpl, err := template.New("resource").Funcs(funcMap).Parse(resourceTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	outPath := fmt.Sprintf("%s/%s_resource.go", outDir, spec.ResourceName)
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
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
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "codegen: "+format+"\n", args...)
	os.Exit(1)
}
