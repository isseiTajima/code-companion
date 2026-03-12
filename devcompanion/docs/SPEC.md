# Sakura Kodama 技術仕様書 (v3.0)

> 最終更新: 2026-03-11

---

## 1. 概要

Sakura Kodama は macOS 上で動作するデスクトップ AI コンパニオン。開発者の作業を常時観測し、状況に応じたセリフをキャラクター（さくら）が吹き出しで表示する。Wails (Go + Svelte) で構築されたスタンドアロン GUI アプリと、WebSocket 経由で接続するサーバーモードの両方をサポートする。

---

## 2. アーキテクチャ：5 層パイプライン

```
[Sensors]
   │  OS・ファイルシステム・Git・Web ブラウザの事実のみを観測
   ▼
[Signals]
   │  意味を持たない低レベルイベント (SigFileModified, SigGitCommit, …)
   ▼
[Context Engine]
   │  複数シグナルの時間密度と重み付けにより開発者の状態を確率的に推定
   ▼
[Monitor / Engine]
   │  状態を MonitorEvent に変換し、セリフ生成の要否を判断
   ▼
[SpeechGenerator]
      プール方式 or 直接生成でセリフを生成し Notifier へ送出
```

---

## 3. 各層の詳細

### 3.1 Sensors

| センサー       | 観測対象                                           |
|--------------|------------------------------------------------|
| ProcessSensor | 実行中プロセス（AI エージェント、IDE など）              |
| FSSensor      | ソースコード・設定ファイルの変更（ファイルパス・拡張子）    |
| GitSensor     | コミット・ブランチ操作（`.git` ディレクトリ監視）          |
| IdleSensor    | キーボード/マウス入力の不在                             |
| WebSensor     | アクティブなブラウザタブの URL（macOS Accessibility API）|

### 3.2 Signals

`types.Signal{Type, Source, Value, Message, Timestamp}` の形式で流通する最小単位のイベント。

| 定数                    | 意味                              |
|------------------------|--------------------------------|
| `SigProcessStarted`    | 新しいプロセスが起動した             |
| `SigProcessStopped`    | プロセスが終了した                  |
| `SigFileModified`      | ファイルが変更された                 |
| `SigManyFilesChanged`  | 短時間に多数のファイルが変更された     |
| `SigGitCommit`         | Git コミットが発生した               |
| `SigLogHint`           | ログファイルから手がかり（成功/失敗）  |
| `SigIdleStart`         | 一定時間操作なし                     |
| `SigSystemWake`        | システムがスリープから復帰した         |
| `SigWebNavigated`      | ブラウザで URL が変わった             |

### 3.3 Context Engine

シグナルの組み合わせと時間密度から `ContextState` を確率的に推定する (`context.Estimator`)。

| 状態                   | 条件                                          |
|----------------------|---------------------------------------------|
| `CODING`             | ファイル変更が継続的に発生                      |
| `AI_PAIR_PROGRAMMING`| AI エージェントプロセスが起動中                 |
| `DEEP_WORK`          | 高密度のファイル変更 + アイドルなし              |
| `STUCK`              | ビルド失敗が頻発                               |
| `PROCRASTINATING`    | ブラウザの非開発 URL への遷移                   |
| `IDLE`               | 一定時間操作なし                               |

### 3.4 Monitor / Engine

Context Engine の出力と各種センサーイベントを `MonitorEvent` に変換し、セリフ生成を `SpeechGenerator` に委譲する。

- `engine.Engine` が全サブシステム（LearningEngine, ProactiveEngine, SituationEngine）を統合する
- `observer.DevObserver` が Git コミットや IDE 観察イベントを別チャンネルで受信する

---

## 4. セリフ生成システム

### 4.1 生成モード

イベントの種類によって **プール方式** または **直接生成** に振り分けられる。

| モード     | 対象 Reason                                      | 特徴                              |
|----------|------------------------------------------------|----------------------------------|
| プール方式 | 上記以外のすべてのイベント                          | 事前バッチ生成・即座に返却          |
| 直接生成   | `user_question`, `web_browsing`, `question_answered` | LLM をリアルタイムで呼び出す        |

### 4.2 プール方式の詳細

#### プールキー

```
"{personality}:{category}:{language}:{timeslot}"
例: "cute:heartbeat:ja:day"
```

- **personality**: `cute` / `genki` / `tsukime`（詳細は §5）
- **category**: `heartbeat` / `working` / `achievement` / `struggle` / `greeting`
- **language**: `ja` / `en`
- **timeslot**: `day`（6:00〜21:59）/ `night`（22:00〜5:59）

timeslot は夜間の語彙・雰囲気を昼間のプールと分離するための分割。

#### バッチ生成フロー

```
1. NeedsRefill(key) が true になったら triggerRefill を goroutine で実行
2. BatchGenerate(BatchRequest) で LLM から 5 件まとめて生成
3. isValidSpeechForLang() でバリデーション（禁止ワード・言語混入チェック）
4. 弾かれたセリフは AddDiscarded(key, speech) に記録
5. 候補が evalKeepCount(2) 件超の場合は evaluateCandidates() で絞り込み
6. 合格したセリフを pool.Push(key, speeches)
```

#### 動的 Avoid リスト

バリデーションで弾かれたセリフを `discarded[key]` に最大 20 件保持し、次回のバッチプロンプトに「NGパターン」として注入する。これにより同じ問題のあるセリフが再生成されにくくなる。

#### クールダウン

バッチ生成したセリフが全件弾かれた場合は `SetCooldown(key, 5min)` を設定し、同じキーへの無駄な再補充を抑制する。

#### 評価 LLM (`evaluator.go`)

有効セリフが `evalKeepCount(2)` 件を超える場合、OllamaClient.GenerateRaw() で低温度（0.1）の評価プロンプトを呼び出し、上位 2 件のインデックスを得て絞り込む。

### 4.3 頻度制御 (FrequencyController)

すべての生成リクエストは `ShouldSpeak()` を通過する。

**alwaysSpeak（クールダウン無視）:**
- `user_click`, `greeting`, `init_setup`
- `dev_session_started`, `ai_session_started`
- `git_commit`, `git_push`
- `user_question`, `question_answered`

**通常クールダウン:** 30 秒

**専用クールダウン:**
- `web_browsing`: 3 分
- `thinking_tick`: SpeechFrequency 設定に応じて 1〜20 分、3 回連続で 15 分のインターバル

---

## 5. パーソナリティシステム

### 5.1 スタイル種別

| ID         | 特徴                                                  |
|------------|-----------------------------------------------------|
| `cute`     | おとなしく献身的。恥ずかしがりで見守り系。デフォルト      |
| `genki`    | エネルギッシュで前向き。成功時・勢いに乗っているとき       |
| `tsukime`  | 直球で正直、少し辛口だが気にかけている。苦戦時・深夜型     |

旧定数 (`soft`/`energetic`/`strict`) は互換性のため残存するが内部では上記にマッピングされる。

### 5.2 自動推論ルール

Config で明示指定がない場合、イベントに応じて以下の優先順位で自動選択される。

1. **tsukime**: ビルド連続失敗 / 同一ファイル 3 回以上編集 / 長時間アイドル
2. **genki**: ビルド成功 / コミット・プッシュ / successStreak ≥ 1
3. **cute**: それ以外（デフォルト）

### 5.3 RelationshipMode

`normal`（デフォルト）/ `lover` の 2 種。`lover` の場合はプロンプトに「少し甘えた口調」の指示が追加される。

---

## 6. 性格学習システム (LearningEngine)

### 6.1 トレイト一覧

トレイトは「どんな質問をするか」を定義するラベルだけを持ち、実際の質問文は LLM が生成する。
ラベルは `locales/ja.yaml` / `en.yaml` の `trait.<id>` キーで管理される。

| TraitID                  | 分類           | ラベル概要                                  |
|--------------------------|--------------|------------------------------------------|
| `focus_style`            | 開発スタイル    | 集中スタイル（一気 vs こまめに休憩）            |
| `feedback_preference`    | 開発スタイル    | どんな声かけが嬉しいか                        |
| `interruption_tolerance` | 開発スタイル    | 作業中に話しかけられたときの感じ方               |
| `debugging_style`        | 開発スタイル    | バグやエラーへの向き合い方                     |
| `code_review_style`      | 開発スタイル    | コードレビューのスタイル（厳しめ vs ゆるめ）      |
| `work_pace`              | 作業リズム      | 集中しやすい時間帯（朝型・夜型など）             |
| `break_style`            | 作業リズム      | 休憩の取り方                                |
| `deadline_style`         | 作業リズム      | 締め切りへの向き合い方                        |
| `learning_style`         | 学習・成長      | 新しいことの学び方（本・動画・実践）             |
| `motivation_source`      | 学習・成長      | やる気が出るきっかけ                          |
| `lifestyle`              | ライフスタイル   | 普段の生活リズムや習慣                        |
| `stress_relief`          | ライフスタイル   | ストレス解消方法                             |
| `music_habit`            | ライフスタイル   | 作業中の音楽習慣                             |
| `food_preference`        | ライフスタイル   | 飲み物・食べ物の好み                          |
| `hobby`                  | 趣味・社交      | 趣味・開発以外で熱中していること                |
| `communication_style`    | 趣味・社交      | コミュニケーションスタイル                     |

新しいトレイトを追加するには:
1. `types/types.go` に `TraitXxx TraitID = "xxx"` 追加
2. `locales/ja.yaml` と `en.yaml` の `trait:` セクションにラベル追加
3. `engine/learning.go` の `ProcessEvent` に適切な好奇心ブーストを追加

### 6.2 好奇心モデル (Curiosity)

各トレイトに 0.0〜1.0 の好奇心スコアを管理する。毎分 `× 0.98` で減衰し、特定のイベントで増加する。

| イベント              | 増加するトレイト                          |
|---------------------|---------------------------------------|
| `StateSuccess/Fail` | `feedback_preference` +0.3            |
| `StateDeepWork`     | `interruption_tolerance` +0.2         |
| `StateCoding`       | `focus_style` +0.15                   |
| `StateIdle`         | `lifestyle` +0.1, `hobby` +0.05       |
| `git_commit/push`   | `work_pace` +0.3                      |
| `idle_start`        | `hobby` +0.2                          |

スコアが `CuriosityThreshold (0.8)` を超え、かつ `MinQuestionInterval (1時間)` が経過し、`MaxQuestionsPerDay (3件)` 未満であれば質問をトリガーする。DeepWork 中は抑制される。

### 6.3 質問の掘り下げ (Stage 進化)

各トレイトは `Confidence` で 3 段階のステージを持つ。

| Stage | Confidence 範囲 | 質問の方針                                |
|-------|--------------|----------------------------------------|
| 0     | 0.0〜0.39    | トレイトについての基本的な質問                |
| 1     | 0.4〜0.79    | 前回の回答を踏まえた掘り下げ質問              |
| 2     | 0.8〜1.0     | さらに具体的な文脈（最近の作業）を絡めた質問   |

**Stage 進化ルール:**
`RecordTraitUpdate()` 呼び出しで `Confidence += 0.2`。`Confidence >= 1.0` で打ち止め。

**掘り下げの仕組み:**
`LastAnswer`（前回の回答テキスト）と `CurrentStage` が質問テンプレートに渡され、同じトレイトをより具体的に掘り下げる質問が生成される。

例:
- Stage 0: 「仕事は集中して一気にやる派ですか？」
- Stage 1, LastAnswer=「一気にやるタイプ」: 「集中が切れたときはどうしてる？」

### 6.4 回答後リアクション

ユーザーが質問に回答すると、`HandleAnswer()` が:
1. `RecordTraitUpdate()` でプロファイルを更新・保存
2. 500ms 待機後、`Generate(reason=question_answered, question=answerText)` を呼び出す

`question_answered` は `alwaysSpeak` のため 30 秒クールダウンを無視する。

セリフ生成テンプレートは `question` フィールドが存在する場合、CoT フローをバイパスして「回答に寄り添った短いリアクション（感謝・共感・メモ）」を生成するモードに切り替わる。

### 6.5 プロファイルの永続化

`~/.config/sakura-kodama/profile.json` に JSON 形式で保存される。保存内容:

- `Personality.Traits`: トレイト ID → float64 (0.0〜1.0)
- `Evolution`: トレイット ID → `TraitProgress{CurrentStage, Confidence, LastAnswer, LastUpdated}`
- `Relationship`: 親密度レベル・信頼度
- `Memories`: プロジェクトの節目 (最大 50 件)
- 開発統計: コミット数・ビルド成功/失敗数・深夜アクティビティ

---

## 7. 積極的介入システム (ProactiveEngine)

外部イベントなしにサクラが自発的に話しかけるシステム。1 分ごとに `Tick()` が呼ばれる。

**発火条件:**
- 最後の介入から `MinInitiativeInterval (15分)` 以上経過
- DeepWork 中でない
- `rand.Float64() <= InitiativeProbability (0.03)` ← 3% の確率

**介入タイプと重み:**

| タイプ          | 重み  | 内容                                  |
|---------------|------|-------------------------------------|
| `observation` | 50%  | 作業内容についての観察コメント            |
| `support`     | 30%  | 労いや応援のメッセージ                   |
| `curiosity`   | 15%  | 話しかけたくなった（軽い質問）            |
| `memory`      | 5%   | 過去のプロジェクトの記憶について言及       |

---

## 8. LLM ルーティング

### 8.1 バックエンド優先順位

```
Ollama (ローカル) → Claude API → Gemini API → ai CLI (レガシー) → Fallback
```

各レイヤーで空レスポンスやエラーが発生した場合に次のレイヤーへフォールバックする。最終フォールバックは YAML の固定フレーズ集 (`speech.fallback.*`) を使用する。

### 8.2 タイムアウト設定

| バックエンド | セリフ生成    | バッチ生成   |
|-----------|------------|-----------|
| Ollama    | 30 秒       | 60 秒      |
| Claude    | 10 秒       | 60 秒      |
| Gemini    | 20 秒       | 60 秒      |
| ai CLI    | 10 秒       | 60 秒      |

### 8.3 Ollama デフォルト設定

- エンドポイント: `http://localhost:11434/api/chat`（`/api/generate` は非推奨）
- デフォルトモデル: `qwen2.5:4b`
- オプション: `temperature=1.0, repeat_penalty=1.3, top_p=0.9, seed=<ランダム>`
- 質問生成時: `temperature=0.7`
- 評価生成時: `temperature=0.1`

### 8.4 セリフバリデーション

生成されたセリフは以下のチェックを通過する必要がある (`isValidSpeechForLang`)。

**日本語モード:**
- ひらがな・カタカナ・漢字が含まれること
- 禁止ワード (「魔法」「ダンス」「宝石」「芸術」「宝物」) を含まないこと

**英語モード:**
- 日本語文字が混入していないこと
- 詩的比喩・サービス業表現の禁止ワード (`blossom`, `spring breeze`, `senpai`, `lovely to see`, など) を含まないこと

---

## 9. プロンプトシステム

### 9.1 テンプレート一覧

| ファイル                      | 用途                                          |
|------------------------------|---------------------------------------------|
| `prompts/ja.tmpl`            | 日本語・通常セリフ生成（CoT 方式）               |
| `prompts/en.tmpl`            | 英語・通常セリフ生成（CoT 方式）                 |
| `prompts/question_ja.tmpl`   | 日本語・性格質問生成（JSON 出力）               |
| `prompts/question_en.tmpl`   | 英語・性格質問生成（JSON 出力）                 |

### 9.2 通常セリフ生成テンプレートの構造 (CoT)

```
[キャラクター] パーソナリティ・関係モードの説明
[現在のイベント] Reason, Behavior, TimeOfDay, Details, Session
[最近の作業メモリ] WorkMemory（存在する場合のみ）
[ルール] 制約事項
[NGフレーズ例] 具体的な禁止表現例

{{if .Question}}
  [指示] 回答テキストへのリアクションを1文で生成（CoT をバイパス）
{{else}}
  [プロセス] 5 段階 CoT:
    1. 観察
    2. 感情を決定
    3. 候補 5 件生成
    4. 最良を選択
    5. 1 件のみ出力
{{end}}
```

### 9.3 バッチ生成テンプレートの構造

YAML (`batch.template`) に定義された文字列テンプレートを `fmt.Sprintf` で展開する。引数順:

```
userName, count, count, userName, personality_desc, mode_desc, category_desc, traits_section, avoid_section
```

`avoid_section` には「最近使ったセリフ」と「NGパターン（動的 Avoid リスト）」の両方が含まれる。

---

## 10. 多言語対応 (i18n)

- すべての表示文言・プロンプト変数は `internal/i18n/locales/ja.yaml` / `en.yaml` で管理する
- Go コード内での if-language 分岐は禁止。`i18n.T(lang, "key")` を使用すること
- `i18n.TVariant(lang, key)` は `[]string` のバリアントリストを返す（Fallback フレーズ用）
- 設定の `language` フィールド: `"ja"` または `"en"`

### 主要な YAML キー構造

```yaml
speech.fallback.<reason>: []   # フォールバック発言（リスト）
speech.greeting.<time>:        # 時間帯別あいさつ
reason.<reason>:               # セリフプロンプト用 Reason の日本語説明
behavior.<behavior>:           # 行動の日本語説明
batch.template:                # バッチ生成用プロンプトテンプレート
batch.personality.<style>:     # パーソナリティ説明
batch.category.<category>:     # カテゴリ説明
batch.avoid_header:            # 避けるべきセリフのヘッダー
batch.discarded_header:        # NG パターンのヘッダー
```

---

## 11. 設定 (Config)

設定ファイル: `~/.config/sakura-kodama/config.yaml`（YAML / JSON 両対応）

| フィールド                  | デフォルト          | 説明                                    |
|--------------------------|-------------------|---------------------------------------|
| `name`                   | `さくら`           | キャラクター名                            |
| `user_name`              | `先輩`             | ユーザー呼称                              |
| `language`               | `ja`              | 言語設定 (`ja` / `en`)                  |
| `persona_style`          | `cute`            | パーソナリティ (`cute`/`genki`/`tsukime`) |
| `relationship_mode`      | `normal`          | 関係モード (`normal` / `lover`)          |
| `model`                  | `qwen2.5:4b`      | Ollama モデル                           |
| `ollama_endpoint`        | `http://localhost:11434/api/generate` | Ollama エンドポイント      |
| `anthropic_api_key`      | `""`              | Claude API キー                         |
| `gemini_api_key`         | `""`              | Gemini API キー                         |
| `llm_backend`            | `""`              | バックエンド強制指定（省略時は自動ルーティング）|
| `speech_frequency`       | `2`               | 発話頻度 (1=少ない, 2=普通, 3=多い)       |
| `monologue`              | `true`            | thinking_tick による独り言を有効にするか   |
| `mute`                   | `false`           | セリフ生成を完全に停止                     |
| `click_through`          | `true`            | クリックをウィンドウに透過させる            |
| `always_on_top`          | `true`            | 最前面表示                               |
| `window_position`        | `top-right`       | 表示位置 (`top-right` / `bottom-right`) |

---

## 12. 通知・トランスポート

### 12.1 GUI モード (Wails)

`transport/wails/notifier.go` が `types.Event` を Wails のランタイムイベントとしてフロントエンドに送出する。フロントエンドは Svelte で実装されており `App.svelte` がイベントを受信してバルーンを表示する。

### 12.2 サーバーモード (WebSocket)

`transport/websocket/notifier.go` が WebSocket 経由でイベントを送出する。`cmd/devcompaniond/main.go` でデーモンとして起動し、ブラウザやカスタムクライアントから接続できる。

### 12.3 Multi Notifier

`transport/multi_notifier.go` で複数のトランスポートに同時送出できる。

---

## 13. UI 仕様

### 13.1 ウィンドウ配置

- **基本位置**: `top-right` または `bottom-right`（設定で変更可能）
- **向き**: 常に画面中央（左側）を向くように水平反転して配置
- **コンテナサイズ**: キャラクター 128px + 吹き出し最大 350px が収まるサイズ

### 13.2 吹き出し (Balloon)

- **相対位置**: キャラクターの左上（画面中央寄り）
- **しっぽの向き**: 吹き出しの右下から、キャラクター頭部に向かって伸びる
- **デザイン**: 白背景（半透明・ぼかし）。AI の成否による色変化は行わない

### 13.3 設定パネル

- ウィンドウ幅 210px 内に収まる縦長のパネル
- ローカルモデルは「カタログリスト」形式で表示（モデル名・サイズ・インストール状態）
- インストール済みはクリックで選択可、未インストールは DL ボタン表示
- DL 中は進捗バー表示。同時実行は 1 件のみ

---

## 14. ディレクトリ構造

```
cmd/
├── contextviewer/     # 状態遷移可視化 CLI ツール
└── devcompaniond/     # サーバーモード (WebSocket デーモン)
internal/
├── agent/             # Agent Adapter インターフェース
├── behavior/          # (Legacy) 行動推論
├── config/            # 設定ロード・保存
├── context/           # Context Engine (状況推定)
├── debug/
│   ├── recorder/      # シグナル JSONL 記録
│   └── replay/        # シグナルリプレイエンジン
├── engine/
│   ├── engine.go      # パイプライン統合
│   ├── learning.go    # 性格学習エンジン
│   ├── proactive.go   # 積極的介入エンジン
│   └── situation.go   # 世界認識モデル
├── i18n/
│   └── locales/       # ja.yaml / en.yaml
├── llm/
│   ├── router.go      # LLM バックエンド切替
│   ├── ollama.go      # Ollama クライアント + バッチ生成
│   ├── claude.go      # Anthropic Claude クライアント
│   ├── gemini.go      # Google Gemini クライアント
│   ├── aicli.go       # ai CLI レガシークライアント
│   ├── speech.go      # SpeechGenerator (メイン)
│   ├── speech_pool.go # プール + 動的 Avoid リスト
│   ├── speech_state.go# 発言履歴・重複管理
│   ├── evaluator.go   # 評価 LLM による絞り込み
│   ├── fallback.go    # Fallback + Reason 定数
│   └── prompts/       # テンプレート (ja/en/question_ja/question_en)
├── memory/            # 作業メモリ (短期コンテキスト)
├── monitor/           # シグナル集約・MonitorEvent 変換
├── observer/          # Git・IDE 観察
├── persona/           # パーソナリティエンジン
├── plugin/            # プラグイン拡張
├── profile/           # 開発者プロファイル永続化
├── sensor/            # OS センサー群
├── session/           # (Legacy) セッション管理
├── transport/         # Wails / WebSocket 通知
├── types/             # 全レイヤー共通の型定義
└── ws/                # WebSocket サーバー
docs/
└── SPEC.md            # 本仕様書
```

---

## 15. デバッグ・観測性

### 15.1 Signal Recorder

`monitor` 層で受信した全シグナルを JSONL 形式で記録。

- **保存場所**: `~/.sakura-kodama/signals/signals_YYYYMMDD_HHMMSS.jsonl`
- **目的**: 本番環境で発生した複雑なシーケンスの記録・再現

### 15.2 Signal Replay Engine

記録されたログをパイプラインに再注入する。

- `RealTime`: 元のイベント間隔を再現
- `Fast`: ウェイトなしで即座に全イベントを処理（ユニットテスト用）

### 15.3 Context Viewer

リプレイ中の内部状態を可視化する CLI ツール。

```sh
go run cmd/contextviewer/main.go -f <log_path>
```

---

## 16. テスト方針

- LLM バックエンドは `httptest.Server` でモック（チャットAPI フォーマット: `{"message":{"content":"..."}}`）
- `SetSeed(42)` で Fallback の乱数を固定して決定論的テスト
- 時刻フィールドは `string` 型（`types.TimeToStr(time.Now())`）
- 長時間安定性テスト: 1 万件のシグナルを高速リプレイし、goroutine リーク・メモリ増大・パニックがないことを検証

---

## 17. 新しいイベント追加手順

1. `types/types.go` に `EventXxx` / `SigXxx` 追加
2. `monitor/monitor.go` の `classifySignal` に追加
3. `llm/fallback.go` に `ReasonXxx` 追加
4. `engine/engine.go` の `reasonFromEvent` にマッピング追加
5. `llm/speech.go` の `poolCategory` にカテゴリ振り分けを追加
6. `locales/ja.yaml` + `en.yaml` に `reason.xxx` / `speech.fallback.xxx` を追加
7. 常時発話が必要な場合は `FrequencyController.ShouldSpeak` の `alwaysSpeak` に追加
