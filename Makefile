ifeq ($(OS),Windows_NT)
    # Windows native setup
    HOMEDIR = $(USERPROFILE)
    # Ensure standard forward-slashes for Makefile variable consistency
    CONFIG_PATH = $(subst \,/,$(HOMEDIR))/.proglog
    MKDIR = powershell -Command "New-Item -ItemType Directory -Force -Path '$(HOMEDIR)\.proglog' | Out-Null"
    MOVE  = powershell -Command "Move-Item -Path *.pem, *.csr, *.csv, *.conf -Destination '$(HOMEDIR)\.proglog\' -Force"
else
    # Linux and macOS setup
    HOMEDIR = $(HOME)
    CONFIG_PATH = $(HOMEDIR)/.proglog
    MKDIR = mkdir -p $(CONFIG_PATH)
    MOVE  = mv *.pem *.csr *.csv *.conf $(CONFIG_PATH)/
endif

.PHONY: init
init:
	$(MKDIR)

.PHONY: gencert
gencert: init
	cfssl gencert \
		-initca auth/ca-csr.json | cfssljson -bare ca
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=auth/ca-config.json \
		-profile=server \
		auth/server-csr.json | cfssljson -bare server
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=auth/ca-config.json \
		-profile=client \
		-cn="client" \
		auth/client-csr.json | cfssljson -bare client
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=auth/ca-config.json \
		-profile=client \
		-cn="intruder" \
		auth/client-csr.json | cfssljson -bare intruder-client
	$(MOVE)

.PHONY: test
test:  $(CONFIG_PATH)/policy.csv $(CONFIG_PATH)/model.conf
	go test -race ./...

# detect any breaking change in proto
lint-breaking::
	go tool buf breaking --against "githubrepo#branch"  #branch could be main or the branch you wanna compare with

# lint the proto file
lint-proto::
	go tool buf lint --config buf.yaml

# generate the Go code from the proto file
generate-proto::
	go tool buf generate --template buf.gen.yaml

# install dependencies. The @ hides the go mod tidy command upon execution
.PHONY: install-tools
install-tools:
	go tool buf dep update
	go mod tidy

# Format the code
format::
	@go tool golangci-lint run --fix -v ./...

.PHONY: docker-comp-up
docker-comp-up:
	cd ./internal/observability && docker compose up -d

.PHONY: docker-comp-down
docker-comp-down:
	cd ./internal/observability && docker compose down

.PHONY: start-server
start-server:
	cd ./cmd/server && go run main.go

.PHONY: start-client
start-client:
	cd ./cmd/client && go run main.go