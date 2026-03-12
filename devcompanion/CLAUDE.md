# CLAUDE.md — Sakura Kodama (devcompanion)

## プロジェクト概要
macOS デスクトップの開発作業をリアルタイム監視し、AIキャラクター「サクラ」がセリフを発するWailsアプリ。
Goバックエンド + Svelteフロントエンド。モジュール名: `sakura-kodama`

## ビルド・テスト
```sh
make test          # 全テスト（Go + フロントエンド）
make test-go       # Go のみ
make dev           # Wails 開発サーバー
go build .         # バイナリビルド確認
```

## アーキテクチャ（5層パイプライン）
```
Sensor → Signal → Context/Behavior → MonitorEvent → SpeechGenerator
```
- `internal/types`     : 全レイヤー共通の型定義（変更時は全体影響を確認）
- `internal/monitor`   : センサー統合・Signal→MonitorEvent変換
- `internal/engine`    : パイプライン統合・LLM呼び出し判断
- `internal/llm`       : LLMルーティング（Ollama→Claude→Gemini→Fallback）
- `internal/i18n`      : 全文言は `locales/ja.yaml` / `en.yaml` に集約（Goコード内にハードコード禁止）

## LLM設計
- **プール方式**: バッチ生成した5件をプールに保持、イベント時に取り出す
- `UserQuestion` / `WebBrowsing` のみ直接LLM生成（リアルタイムコンテキスト必要）
- `BatchRequest{Personality, Category, Language, UserName, Count, RecentLines}`
- PoolKey: `"cute:heartbeat:ja"` 形式
- Fallbackは `internal/llm/fallback.go` の `FallbackSpeech(reason, lang)`

## 多言語対応
- 文言・プロンプト・バッチテンプレートは全て `locales/*.yaml` で管理
- Go内でのif-language分岐は禁止。`i18n.T(lang, "key")` を使う

## テスト方針
- Ollama/Claude はhttptest.Serverでモック（チャットAPIフォーマット: `{"message":{"content":"..."}}` ）
- `SetSeed(42)` で乱数を固定してFallback出力を決定論的にする
- 時刻フィールドは `string` 型（`types.TimeToStr(time.Now())`）

## 新しいイベント追加手順
1. `types.go` に `EventXxx` / `SigXxx` 追加
2. `monitor/monitor.go` の `classifySignal` に追加
3. `fallback.go` に `ReasonXxx` 追加
4. `engine.go` の `reasonFromEvent` にマッピング追加
5. `speech.go` でプール or 直接生成を選択
6. `locales/ja.yaml` + `en.yaml` に `reason.xxx` / `speech.fallback.xxx` 追加
