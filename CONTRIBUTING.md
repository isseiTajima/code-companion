# Contributing to Sakura Kodama

Thanks for your interest. Here's what you need to know before contributing.

## Before you start

For anything beyond a small bug fix, please open an issue first to discuss the direction. This project has intentional design constraints — especially around character behavior — and it helps to align early.

## Setup

```bash
git clone https://github.com/isseiTajima/sakura-kodama
cd sakura-kodama/devcompanion

# Install frontend dependencies
cd frontend && npm install && cd ..

# Run in development mode
wails dev
```

Requirements: Go 1.21+, Node.js 18+, [Wails v2](https://wails.io), Ollama (optional, for local LLM)

## Running tests

```bash
make test        # Go + frontend
make test-go     # Go only
```

All tests must pass before opening a PR.

## What to keep in mind

**Character integrity**
Sakura's tone and behavior are intentional. She is observant, occasionally present, and never intrusive. Changes to speech style, personality, or interaction patterns should fit that shape. When in doubt, less is more.

**Architecture**
The codebase is layered: sensors → context → inner state → speech output. Keep concerns in their layer. Don't reach across layers for convenience.

**Strings and i18n**
All user-facing strings go in `internal/i18n/locales/ja.yaml` and `en.yaml`. No hardcoded strings in Go code. Use `i18n.T(lang, "key")`.

**Tests**
- Mock Ollama/Claude with `httptest.Server`
- Use `SetSeed(42)` to make fallback output deterministic
- Time-sensitive tests must use fixed timestamps (not `time.Now()`)

## Adding a new event type

See `devcompanion/CLAUDE.md` for the step-by-step checklist.

## Pull requests

Use the PR template. Keep changes focused — one concern per PR.
