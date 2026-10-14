BIN := bin/goinv
RUN := go run ./cmd/goinv
DAYS ?= 30

.PHONY: run build install test vet fmt logout status snapshot history clean

run:
	@$(RUN)

build:
	go build -o $(BIN) ./cmd/goinv

install:
	go install ./cmd/goinv
	@dir="$$(go env GOBIN)"; dir="$${dir:-$$(go env GOPATH)/bin}"; \
	echo "goinv instalado en $$dir/goinv"; \
	case ":$$PATH:" in *":$$dir:"*) ;; *) echo "Esa carpeta no está en tu PATH; agrégala con: export PATH=\"$$dir:\$$PATH\"";; esac

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

logout:
	@$(RUN) logout

status:
	@$(RUN) status

# Non-interactive, meant for cron.
snapshot:
	@$(RUN) snapshot

history:
	@$(RUN) history $(DAYS)

clean:
	rm -rf bin
