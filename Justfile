#!/usr/bin/env just --justfile

default:
    @just --list

# ----------------------------------------------------------------
# Development
# ----------------------------------------------------------------

# Recursively analyze directory tree structure with file filtering, tree view, colors, and verbose stats output
[group('Development')]
treeclip dir="":
    treeclip run {{ dir }} -f -t -c -v --stats

# Compile the Go generator application into an executable binary at ./bin/doppelgen
[group('Development')]
build-doppelgen:
    go build -o bin/doppelgen ./cmd/doppelgen

# Run all Go tests with verbose output to show each test's progress and results
[group('Development')]
test:
    go test -v ./...

# Generate a coverage profile and open an HTML visualization showing which code lines are tested
[group('Development')]
test-coverage:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out

# ----------------------------------------------------------------
# Benchmark
# ----------------------------------------------------------------

# Run benchmarks for all packages with memory allocation stats
[group('Benchmark')]
bench pattern=".":
    go test -bench='{{ pattern }}' -benchmem ./...

# Run benchmarks multiple times for statistical stability (count parameter allows variable runs)
[group('Benchmark')]
bench-count pattern="Benchmark" count="3":
    go test -bench='{{ pattern }}' -benchmem -count={{ count }} ./...

# Run benchmarks and save results to a file for later analysis with benchstat
[group('Benchmark')]
bench-save pattern="Benchmark" count="5" output="bench.txt":
    go test -bench='{{ pattern }}' -benchmem -count={{ count }} ./... > {{ output }}

# Compare benchmark result file statistically using benchstat (requires golang.org/x/perf/cmd/benchstat)
[group('Benchmark')]
benchstat file="bench.txt":
    benchstat {{ file }} && benchstat {{ file }} > benchstat.txt

# Compare two benchmark result files using benchstat (requires golang.org/x/perf/cmd/benchstat)
[group('Benchmark')]
benchstat-cmp old="old.txt" new="new.txt":
    benchstat {{ old }} {{ new }} && benchstat {{ old }} {{ new }} > benchstat.txt

# Run quick benchmark with default pattern (all benchmarks) and 1 iteration
[group('Benchmark')]
bench-quick pattern=".":
    go test -bench='{{ pattern }}' -benchtime=1x ./...

# ----------------------------------------------------------------
# Code Quality
# ----------------------------------------------------------------

# Run golangci-lint to identify potential bugs, style issues, and performance problems in the codebase
[group('Code Quality')]
lint:
    golangci-lint run

# Automatically format all Go source files according to standard Go formatting rules (gofmt style)
[group('Code Quality')]
fmt:
    go fmt ./...

# Analyze Go source code for suspicious constructs, unreachable code, and other potential errors
[group('Code Quality')]
vet:
    go vet ./...

# Run advanced static analysis tool that catches bugs, simplifies code, and enforces best practices
[group('Code Quality')]
staticcheck:
    staticcheck ./...

# Run all code quality checks sequentially: formatting, vetting, linting, and static analysis
[group('Code Quality')]
all-checks: fmt vet lint staticcheck

# ----------------------------------------------------------------
# Dependency
# ----------------------------------------------------------------

# Download all required Go module dependencies to the local module cache
[group('Dependency')]
mod-download:
    go mod download

# Clean up go.mod and go.sum by removing unused dependencies and adding missing ones
[group('Dependency')]
mod-tidy:
    go mod tidy

# Copy all module dependencies into a local vendor directory for offline builds
[group('Dependency')]
mod-vendor:
    go mod vendor

# Remove the vendor directory and clear the entire Go module cache to force fresh downloads
[group('Dependency')]
mod-clean:
    rm -rf ./vendor
    go clean -modcache

# Update all direct and indirect module dependencies to their latest minor/patch versions
[group('Dependency')]
mod-update:
    go get -u ./...
    go mod tidy

# ----------------------------------------------------------------
# Docs
# ----------------------------------------------------------------

# Generate navigation HTML for documentation markdown files
[group('Docs')]
generate-nav:
    @echo "🔄 Generating documentation navigation..."
    @go run scripts/generate-nav.go
    @echo "✨ Navigation updated! Commit the changes."

# Check if navigation HTML needs to be updated
[group('Docs')]
check-nav: generate-nav
    @git diff --exit-code docs/*.md || (echo "❌ Navigation changes detected. Run 'just generate-nav' and commit." && exit 1)
    @echo "✅ Documentation navigation is up to date."

# ----------------------------------------------------------------
# Git & Version Control
# ----------------------------------------------------------------

# Stage all current changes and add them to the most recent commit, editing the commit message
[group('Git')]
amend:
    git commit -a --amend

# Create a new commit with no file changes, useful for triggering CI or adding metadata
[group('Git')]
empty:
    git commit --allow-empty

# Interactively rebase the last N commits to edit, squash, reorder, or drop them
[group('Git')]
rebase n="3":
    git rebase -i HEAD~{{ n }}

# Copy the current unstaged/staged diff to the system clipboard for sharing or documentation
[group('Git')]
[linux]
diff-cp:
    git diff | xclip -selection clipboard

# Copy current diff to clipboard.
[group('Git')]
[windows]
diff-cp:
    git diff HEAD | /c/Windows/System32/clip.exe

# Copy all logs in graph mode.
[group('Git')]
[linux]
log-cp:
    git log --graph --all | xclip -selection clipboard

# Copy all logs in graph mode.
[group('Git')]
[windows]
log-cp:
    git log --graph --all | clip

# Show all (onelined) commit titles made since midnight for daily standup or activity tracking
[group('Git')]
today:
    git log --since="today 00:00:00" --until="today 23:59:59" --oneline | xclip -selection clipboard

# Show all (detailed) commit titles made since midnight for daily standup or activity tracking
[group('Git')]
today-all:
    git log --since="today 00:00:00" --until="today 23:59:59" | xclip -selection clipboard
