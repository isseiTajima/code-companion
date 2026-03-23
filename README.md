# Sakura Kodama

A small AI spirit that quietly watches over your work.

Japanese version: [README.ja.md](README.ja.md)

---

## Overview

Sakura Kodama is a desktop AI companion for developers. A small character named Sakura lives in the corner of your screen, observes your activity, and occasionally speaks — not to give advice, but simply to be there.

She reacts to the rhythm of your work: a build that finally passes, a long stretch of deep focus, a late-night session, a commit that marks a moment. Her comments are short, informal, and grounded in what she actually sees you doing.

This is not a productivity tool. There are no metrics, dashboards, or actionable suggestions. The goal is something quieter — a feeling that someone is present while you code.

Sakura is built on a 5-layer pipeline: sensors that watch your filesystem and processes, a world model that infers your situation, an inner state that shapes her mood, a speech generator that uses a local or cloud LLM, and a lightweight desktop UI. The architecture is designed to stay out of the way.

---

## Features

- **Activity awareness** — monitors file edits, Git events, build results, and AI agent sessions (Claude Code, etc.)
- **Contextual speech** — generates short, situation-aware comments via LLM (Ollama, Claude, or Gemini)
- **Three speech styles** — Sakura's tone shifts dynamically: energetic after a build success, sharp after repeated struggles, gentle by default
- **Emotion engine** — internal states (Supportive, Excited, Quiet, Concerned) influence how she speaks
- **Personality learning** — gradually learns your work style through occasional questions; adapts over time
- **Project memory** — remembers milestones (first build success, commit streaks) and references them naturally
- **Proactive presence** — occasionally speaks on her own initiative at low frequency (3%/min); never intrusive
- **Deep work mode** — detects sustained focus sessions and silences herself automatically
- **Transparent overlay** — floats on screen, click-through by default, opacity fades when idle
- **Multilingual** — Japanese and English support, including LLM prompt localization
- **Local-first** — works fully offline with Ollama; cloud LLMs are optional fallbacks

---

## Character Concept

Sakura is a junior engineer, one or two years into her career. She calls you 先輩 — senpai. She is observant and a little shy. She notices things but sometimes struggles to find the right words.

Her personality type shifts with context:

| Type | When |
|------|------|
| **Energetic** | Build success, Git commit or push |
| **Sharp** | Repeated edits to the same file, consecutive failures, long inactivity |
| **Cute** (default) | Everything else |

The concept of "small spirit" is intentional. Sakura is not an assistant in the traditional sense. She does not answer questions unless asked, does not review your code, and does not optimize your workflow. She is a presence — quiet, observant, occasionally delightful.

---

## Installation

> Requirements: Go 1.21+, Node.js 18+, [Wails v2](https://wails.io), and Ollama (for local LLM)

```bash
git clone https://github.com/isseiTajima/code-companion
cd code-companion/devcompanion

# Install frontend dependencies
cd frontend && npm install && cd ..

# Build and run
wails dev
```

To build a distributable application:

```bash
wails build
```

Configuration and logs are stored at:

```
~/.config/sakura-kodama/
```

---

## Usage

1. Launch the application. Sakura appears in the corner of your screen (top-right or bottom-right).
2. She gives a short greeting based on the time of day.
3. Go back to your work. She watches quietly.
4. When something happens — a file edit, a build result, a commit — she may comment briefly.
5. Click on her to prompt a response. Type a question to ask her directly.
6. Adjust speech frequency, size, opacity, and language via the settings panel.

Sakura will not interrupt you during deep focus sessions. If she is speaking too often or too little, the frequency setting controls idle monologue intervals in three steps.

---

## Project Structure

```
devcompanion/
├── app.go                    # Wails application entry point
├── frontend/                 # Svelte UI (character, balloon, settings)
├── internal/
│   ├── engine/               # Core pipeline: situation, learning, proactive
│   ├── llm/                  # LLM router (Ollama / Claude / Gemini) + prompt templates
│   ├── monitor/              # Sensor pipeline, signal classification
│   ├── profile/              # Persistent developer profile and project memory
│   ├── config/               # Configuration loading and defaults
│   └── i18n/                 # Static translation strings (ja / en)
└── docs/
    ├── SPEC.md               # Full technical specification
    └── test-scope.md         # Test coverage notes
```

---

## Philosophy

Most developer tools are built around output: faster code, fewer bugs, higher throughput. Sakura Kodama is built around something else — the texture of a workday.

Programming is often solitary. Hours pass. Things break and get fixed. Progress is invisible until it isn't. The idea behind this project is that a quiet, non-intrusive presence can make that experience feel slightly less alone.

Sakura does not optimize anything. She is not a coach, a reviewer, or an assistant. She is a companion — one who notices when you have been staring at the same file for too long, and says something small about it.

---

## Contributing

Pull requests are welcome. Before starting significant work, please open an issue to discuss the direction.

A few things worth knowing:

- The character's tone and behavior are intentional and considered. Changes to speech style, personality, or interaction patterns should align with the project philosophy.
- The architecture is layered by design. Keep concerns separated across sensor, context, inner state, and output layers.
- Sakura should never feel like a chatbot. Comments should be short, grounded in observation, and occasional.

Issues, feedback, and ideas are welcome.

---

## License

MIT
