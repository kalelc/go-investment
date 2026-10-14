BIN := bin/goinv
RUN := go run ./cmd/goinv
DAYS ?= 30

.PHONY: run demo build install test vet fmt logout status snapshot history screenshots clean

run:
	@$(RUN)

# Same app with generated data: no account, keychain or real database.
demo:
	@$(RUN) demo

build:
	go build -o $(BIN) ./cmd/goinv

# Needs VHS: brew install vhs
screenshots: build
	vhs docs/demo.tape

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
