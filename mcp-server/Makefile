###############################################################################
# Setup for development
###############################################################################
.PHONY: install
install:
	brew install lefthook
	lefthook install
	go get -tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4


###############################################################################
# Golang Linting
###############################################################################
# 2.5s as of Feb, 2026
.PHONY: go-fmt
go-fmt:
	go tool golangci-lint fmt

# lint
# 65.38s as of Feb, 2026
.PHONY: go-lint-check
go-lint-check:
	go tool golangci-lint run

# lint and fix
.PHONY: go-lint
go-lint:
	go tool golangci-lint run --fix

# 6.12s as of Feb, 2026
.PHONY: go-lint-fast-check
go-lint-fast-check:
	go tool golangci-lint run --fast-only

.PHONY: go-lint-fast
go-lint-fast:
	go tool golangci-lint run --fast-only --fix

# verify golangci-lint configuration
# Note: run after modifying .golangci.yml
.PHONY: go-lint-verify-config
go-lint-verify-config:
	go tool golangci-lint config verify

# clean golangci-lint cache
.PHONY: go-clean-lint-cache
go-clean-lint-cache:
	go tool golangci-lint cache clean

###############################################################################
# Golang Test
###############################################################################
.PHONY: go-test
go-test:
	go test -timeout 120s ./...

#------------------------------------------------------------------------------
# GoDoc
#------------------------------------------------------------------------------
.PHONY: install-godoc
install-godoc:
	go install golang.org/x/tools/cmd/godoc@latest

# access to 
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/analytics/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/ast/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/dashboard/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/events/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/history/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/indexer/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/profile/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/prompt/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/search/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/state/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/tools/
# http://localhost:6060/pkg/github.com/hiromaily/claude-forge/mcp-server/internal/validation/

.PHONY: run-godoc
run-godoc:
	godoc -http=:6060
