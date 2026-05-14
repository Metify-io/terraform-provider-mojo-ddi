BINARY     := terraform-provider-mojo-ddi
VERSION    ?= 0.1.0-dev
GOFLAGS    := -trimpath
LDFLAGS    := -s -w -X main.version=$(VERSION)

.PHONY: build install test testacc lint generate validate clean

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/terraform-provider-mojo-ddi

install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/metify/mojo-ddi/$(VERSION)/linux_amd64
	cp $(BINARY) ~/.terraform.d/plugins/registry.terraform.io/metify/mojo-ddi/$(VERSION)/linux_amd64/

test:
	go test -v -count=1 ./...

testacc:
	TF_ACC=1 go test -v -count=1 -timeout 120m ./...

lint:
	golangci-lint run ./...

generate:
	go run ./tools/codegen -schema docs/mcp-tool-schemas/ipam.json -out internal/resources/ipam/
	go run ./tools/codegen -schema docs/mcp-tool-schemas/ddi.json -out internal/resources/ddi/

validate:
	go run ./tools/codegen -schema docs/mcp-tool-schemas/ipam.json -out internal/resources/ipam/ -validate
	go run ./tools/codegen -schema docs/mcp-tool-schemas/ddi.json -out internal/resources/ddi/ -validate

clean:
	rm -f $(BINARY)
	go clean -testcache
