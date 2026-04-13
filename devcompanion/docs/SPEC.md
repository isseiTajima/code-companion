# Sakura Kodama 技術仕様書 (v4.0)

> 最終更新: 2026-04-11

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
| AIAgentSensor | AI エージェントプロセスの起動・停止（claude/gemini/copilot/windsurf/aider 等）|
| FSSensor      | ソースコード・設定ファイルの変更（ファイルパス・拡張子）    |
| GitSensor     | コミット・ブランチ操作（`.git` ディレクトリ監視）          |
| IdleSensor    | キーボード/マウス入力の不在                             |
| WebSensor     | アクティブなブラウザタブの URL（macOS AppleScript 経由）。10 秒間同一 URL に滞在した場合のみ発火（`dwellThreshold=2`）。検索結果・広告・ブラウザ内部ページ（`chrome://` 等）は `isNoisyURL()` でフィルタ除外 |

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
"{personality}:{category}:{language}"
例: "cute:heartbeat:ja"
```

- **personality**: `cute` / `genki` / `cool` / `oneesan`（詳細は §5）
- **category**: `heartbeat` / `working` / `achievement` / `struggle` / `greeting`
- **language**: `ja` / `en`

#### バッチ生成フロー

```
1. NeedsRefill(key) が true になったら triggerRefill を goroutine で実行
2. BatchGenerate(BatchRequest) で LLM から 10 件まとめて生成
3. isValidSpeechForLang() でバリデーション（禁止ワード・言語混入チェック）
4. 弾かれたセリフは AddDiscarded(key, speech) に記録
5. 候補が evalKeepCount(3) 件超の場合は evaluateCandidates() で絞り込み
6. 合格したセリフを pool.Push(key, speeches)
```

#### 起動時プリウォーム

`NewSpeechGenerator()` 呼び出し時に `prewarmPools()` を goroutine で実行する。

- `cfg.PersonaStyle` が `cute`/`genki`/`cool`/`oneesan` のいずれかに明示設定されている場合 → **そのペルソナのみ**をプリウォーム（未使用ペルソナの LLM 呼び出しを削減）
- `PersonaStyle` が未設定の場合 → `PersonalityManager` が動的に cute/cool/genki を遷移するため 3 種をプリウォーム
- 将来ライブ切り替えを実装する際は、設定変更イベント受信時に新ペルソナの refill をトリガーする形で拡張する

#### 動的 Avoid リスト

バリデーションで弾かれたセリフを `discarded[key]` に最大 20 件保持し、次回のバッチプロンプトに「NGパターン」として注入する。これにより同じ問題のあるセリフが再生成されにくくなる。

#### クールダウン

バッチ生成したセリフが全件弾かれた場合は `SetCooldown(key, 5min)` を設定し、同じキーへの無駄な再補充を抑制する。

#### 評価 LLM (`evaluator.go`)

有効セリフが `evalKeepCount(3)` 件を超える場合、OllamaClient.GenerateRaw() で低温度（0.1）の評価プロンプトを呼び出す。「本当に人間が言いそうな言葉か（定型文・AIっぽい表現でない）」「直近セリフと被らない」「トーン・言い回しのバリエーション」の3基準でOllamaが上位3件のインデックスを返す。

### 4.3 品質パイプライン

バッチ生成後・プール投入前に Processor → Validator の2段階で処理される。

#### Processor (`processor.go`)
| Processor | 動作 |
|-----------|------|
| `PhraseScrubProcessor` | 「応援してます」など口癖化フレーズをインライン除去 |
| `LengthLimitProcessor` | 90字超えをその場でトリミング |
| `SentenceTrimProcessor` | 2文以上あれば最初の1文に切り詰め |

#### Validator (`validator.go`)
| Validator | 動作 |
|-----------|------|
| `LengthValidator{MinLength:5}` | 5文字未満を reject |
| `SuspectWordValidator` | 15文字超の1単語を reject（HTML/コード混入対策）|
| `ScriptValidator` | 中国語等の混入を reject |
| `LanguageConsistencyValidator` | 言語不一致を reject |
| `BannedWordValidator` | 「集中」「休憩」「コーヒー」「お疲れ様」「無理しないでください」等を reject |
| `RegexValidator` | 誤語尾（わよ/わね/やろ/なさい等）を reject |

#### 監査ログ (`audit.go`)
- 保存場所: `~/.sakura-kodama/audit/speech_YYYYMMDD.jsonl`
- type: `speech`（表示）/ `rejected`（直接生成reject）/ `batch_rejected`（バッチreject）
- 日次レポート: `scripts/audit_report.sh`

### 4.4 頻度制御 (FrequencyController)

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

| ID        | 特徴                                                  |
|-----------|-----------------------------------------------------|
| `cute`    | 技術はよくわからないが、そばで応援している。おとなしく献身的。デフォルト |
| `genki`   | 全力で応援。エネルギッシュで前向き |
| `cool`    | 直球で正直、少し辛口だが気にかけている |
| `oneesan` | 落ち着いたお姉さん。フィラーなし。穏やかで頼れる |

### 5.2 自動推論ルール

Config で明示指定がない場合、イベントに応じて以下の優先順位で自動選択される。

1. **cool**: ビルド連続失敗 / 同一ファイル 3 回以上編集 / 長時間アイドル
2. **genki**: ビルド成功 / コミット・プッシュ / successStreak ≥ 1
3. **cute**: それ以外（デフォルト）

### 5.3 UI Tone → PersonaStyle マッピング

設定UIの `tone` フィールドが変更されると `SaveConfig` 内で自動的に `PersonaStyle` に変換される。

| tone (UI) | PersonaStyle | キャラ像 |
|-----------|-------------|---------|
| `genki`   | `genki`     | 元気・全力応援 |
| `calm`    | `cute`      | おとなしめ・献身的 |
| `oneesan` | `oneesan`   | 落ち着いたお姉さん |
| `tsundere`| `cool`      | 直球・ちょっとツン |

### 5.4 RelationshipMode

`normal`（デフォルト）/ `lover` の 2 種。`lover` の場合はプロンプトに「少し甘えた口調」の指示が追加される。

---

## 6. 性格学習システム (LearningEngine)

### 6.1 トレイト一覧

トレイトは「どんな質問をするか」を定義するラベルだけを持ち、実際の質問文は LLM が生成する。
ラベルは `locales/ja.yaml` / `en.yaml` の `trait.<id>` キーで管理される。

トレイトは2種に分類される。**質問対象**は `allTraits` リストに含まれ実際に質問が発生する。**定義のみ**は types.go に存在するが「技術のことはわからないさくら」のキャラ設定と合わないため `allTraits` から除外されている。

**質問対象トレイト（`allTraits` に含まれるもの）**

| TraitID                   | 分類          | ラベル概要                                     |
|---------------------------|-------------|---------------------------------------------|
| `thinking_style`          | 思考          | 直感で動く vs 論理的に整理してから動く              |
| `curiosity_level`         | 思考          | 新しいものへの興味の湧きやすさ                    |
| `perfectionism`           | 思考          | 完璧主義 vs まず動くものを出す                    |
| `learning_style`          | 学習・成長     | 新しいことの学び方（本・動画・実践）                |
| `motivation_source`       | 学習・成長     | やる気が出るきっかけ                             |
| `teaching_style`          | 学習・成長     | 人に教えることへのスタンス                        |
| `workspace_style`         | 環境          | デスク周りの整理具合                             |
| `notification_style`      | 環境          | 通知の確認スタイル（即確認 vs まとめて）            |
| `silence_preference`      | 環境          | 静かな環境 vs 音があっても OK                    |
| `background_noise`        | 環境          | カフェ・人のいる場所での作業の好み                 |
| `encouragement_style`     | メンタル       | 落ち込んだときの励まされ方の好み                  |
| `praise_preference`       | メンタル       | 褒められることへの感じ方                         |
| `stress_relief`           | メンタル       | ストレス解消方法                                |
| `alone_preference`        | メンタル       | 一人でいる時間の好み                             |
| `lifestyle`               | ライフスタイル  | 普段の生活リズムや習慣                           |
| `music_habit`             | ライフスタイル  | 作業中の音楽習慣                                |
| `food_preference`         | ライフスタイル  | 飲み物・食べ物の好み全般                         |
| `favorite_drink`          | ライフスタイル  | 作業中によく飲むもの                             |
| `favorite_snack`          | ライフスタイル  | 作業中のおやつ・間食習慣                         |
| `hobby`                   | 趣味          | 開発以外で熱中していること・休日の過ごし方          |
| `game_preference`         | 趣味          | ゲームのジャンル・よくやるタイトル                 |
| `anime_preference`        | 趣味          | アニメ・マンガの好み                             |
| `reading_habit`           | 趣味          | 読書習慣（小説・マンガ・エッセイなど）              |
| `communication_style`     | 人間関係       | ひとりで進める vs 話しながら進める                 |
| `conversation_style`      | 人間関係       | 話すのが好き vs 聞くのが好き                     |
| `personality_attraction`  | 人間関係       | 惹かれる人のタイプ・雰囲気                        |
| `favorite_season`         | 雑談          | 好きな季節とその理由                             |
| `favorite_weather`        | 雑談          | 好きな天気・気候                                |

**定義のみ（質問しない）:** `debugging_style`, `code_review_style`, `experiment_style`, `tech_interest`, `focus_style`, `feedback_preference`, `interruption_tolerance`, `work_pace`, `break_style`, `deadline_style`, `multitask_style`, `risk_tolerance` — 技術・開発作業固有の話題はさくらのキャラ設定（技術がわからない後輩）と合わず、セリフ生成にも役立たないため除外。

新しいトレイトを追加するには:
1. `types/types.go` に `TraitXxx TraitID = "xxx"` 追加
2. `locales/ja.yaml` と `en.yaml` の `trait:` セクションにラベル追加
3. `engine/learning.go` の `ProcessEvent` に適切な好奇心ブーストを追加

### 6.2 好奇心モデル (Curiosity)

各トレイトに 0.0〜1.0 の好奇心スコアを管理する。毎分 `× 0.98` で減衰し、特定のイベントで増加する。

| イベント              | 増加するトレイト                                  |
|---------------------|-----------------------------------------------|
| `StateSuccess/Fail` | `encouragement_style` +0.3                    |
| `StateDeepWork`     | `silence_preference` +0.2                     |
| `StateCoding`       | `thinking_style` +0.15                        |
| `StateIdle`         | `lifestyle` +0.1, `hobby` +0.05               |
| `git_commit/push`   | `motivation_source` +0.3                      |
| `idle_start`        | `hobby` +0.2                                  |

> 注: 好奇心ブーストの対象トレイットは `allTraits` に含まれるもののみ有効。`focus_style` 等の除外トレイットへのブーストは learning.go 内に残っているが、質問トリガーには影響しない（スコアが閾値を超えても `allTraits` 外のため質問が生成されない）。

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
- 最後の介入から `MinInitiativeInterval (6分)` 以上経過
- DeepWork 中でない
- `rand.Float64() <= InitiativeProbability (0.03)` ← 3% の確率

**介入タイプと重み:**

| タイプ          | 重み  | 内容                                  |
|---------------|------|-------------------------------------|
| `observation` | 45%  | 作業内容についての観察コメント            |
| `support`     | 27%  | 労いや応援のメッセージ                   |
| `curiosity`   | 15%  | 話しかけたくなった（個人的な話題）         |
| `weather`     | 8%   | 今日の天気・昨日との気温差に触れる         |
| `memory`      | 5%   | 過去のプロジェクトの記憶について言及       |

**`initiative_curiosity` の制約:** 技術・ライブラリ・コード・作業内容には一切触れず、先輩の日常・趣味・気分・好みなど個人的な話題のみを扱う（プロンプトに `[重要]` セクションで明示）。

---

## 8. LLM ルーティング

### 8.1 バックエンド優先順位

```
Ollama (ローカル) → Claude API → Gemini API → ai CLI (レガシー) → Fallback
```

各レイヤーで空レスポンスやエラーが発生した場合に次のレイヤーへフォールバックする。最終フォールバックは YAML の固定フレーズ集 (`speech.fallback.*`) を使用する。

### 8.5 接続障害検知

`LLMRouter` は全バックエンドが連続失敗した回数を `failStreak`（`atomic.Int32`）で追跡する。

- 全バックエンド失敗時: `failStreak++`
- いずれかのバックエンドが成功時: `failStreak = 0`
- `failStreak >= 3` かつ前回警告から **30 分以上**経過している場合: `consumeConnWarn()` が `true` を返し、通常の fallback セリフの代わりに接続確認を促すセリフ（`connWarnSpeech()`）を表示する

対象は `StrategyDirect`（直接生成）の失敗のみ。プール補充失敗（`BatchGenerate`）はカウントしない。

### 8.2 タイムアウト設定

| バックエンド | セリフ生成    | バッチ生成   |
|-----------|------------|-----------|
| Ollama    | 30 秒       | 60 秒      |
| Claude    | 10 秒       | 60 秒      |
| Gemini    | 20 秒       | 60 秒      |
| ai CLI    | 10 秒       | 60 秒      |

### 8.3 Ollama デフォルト設定

- エンドポイント: `http://localhost:11434/api/chat`
- デフォルトモデル: `qwen3.5:9b`
- オプション: `temperature=1.0, repeat_penalty=1.3, top_p=0.9, seed=<ランダム>`
- 質問生成時: `temperature=0.7`
- 評価生成時: `temperature=0.1`

### 8.4 セリフバリデーション

詳細は §4.3 品質パイプライン を参照。バリデーション設定は `internal/llm/validator.go` で管理される。

---

## 8a. 天気履歴システム

### 概要

`internal/weather/history.go` が日次の天気記録を保持し、発言時に「昨日・先週との差分」を文脈として付与する。

### 保存仕様

- **保存場所**: `~/.sakura-kodama/weather_history.json`
- **形式**: `{"records": [{"date": "2006-01-02", "temp_c": 18, "desc": "Sunny"}, ...]}`
- **保持件数**: 最大 31 件（古い日付から削除）
- **更新タイミング**: `Fetcher.Get()` が新規フェッチに成功するたびに当日レコードを upsert

### 差分コンテキスト (`DeltaContext`)

| 比較対象 | 条件                                       | 出力例                   |
|--------|------------------------------------------|------------------------|
| 昨日    | 前日のレコードが存在する場合（優先）            | `昨日より5°C暖かい`       |
| 先週    | 昨日のレコードがなく7日前のレコードが存在する場合 | `先週より少し寒い`         |
| なし    | 比較対象なし・差分が ±1°C 以内               | `""（空文字、付与しない）` |

**温度差の閾値:**
- `diff >= 5°C`: `"より{N}°C暖かい"`
- `2 ≤ diff < 5°C`: `"より少し暖かい"`
- `-1 ≤ diff ≤ 1°C`: `"とほぼ同じ気温"`
- `-4 < diff ≤ -2°C`: `"より少し寒い"`
- `diff ≤ -5°C`: `"より{N}°C寒い"`

### LLM へのコンテキスト注入

`ProactiveEngine` の `InitWeather` ハンドラが `WeatherContext` を構築する:

```
"Tokyo: Sunny, 22°C（昨日より5°C暖かい）"
```

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
| `persona_style`          | `cute`            | パーソナリティ (`cute`/`genki`/`cool`/`oneesan`) |
| `relationship_mode`      | `normal`          | 関係モード (`normal` / `lover`)          |
| `model`                  | `qwen3.5:9b`      | Ollama モデル                           |
| `ollama_endpoint`        | `http://localhost:11434/api/chat` | Ollama エンドポイント      |
| `anthropic_api_key`      | `""`              | Claude API キー                         |
| `gemini_api_key`         | `""`              | Gemini API キー                         |
| `llm_backend`            | `""`              | バックエンド強制指定（省略時は自動ルーティング）|
| `speech_frequency`       | `2`               | 発話頻度 (1=少ない, 2=普通, 3=多い)       |
| `monologue`              | `true`            | thinking_tick による独り言を有効にするか   |
| `mute`                   | `false`           | セリフ生成を完全に停止                     |
| `click_through`          | `true`            | クリックをウィンドウに透過させる            |
| `always_on_top`          | `true`            | 最前面表示                               |
| `window_position`        | `top-right`       | 表示位置 (`top-right` / `bottom-right`) |
| `tone`               | `calm`            | UI口調設定 (`genki`/`calm`/`oneesan`/`tsundere`) |
| `news_enabled`       | `false`           | ニュース取得を有効にするか |
| `weather_enabled`    | `false`           | 天気取得を有効にするか |

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
├── config/
│   ├── config.go      # 設定ロード・保存
│   └── manager.go     # ConfigManager（保存・ロードのラッパー）
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
│   ├── ollama_manager.go  # モデルのpull/create/delete管理
│   ├── claude.go      # Anthropic Claude クライアント
│   ├── gemini.go      # Google Gemini クライアント
│   ├── aicli.go       # ai CLI レガシークライアント
│   ├── speech.go      # SpeechGenerator (メイン)
│   ├── speech_pool.go # プール + 動的 Avoid リスト
│   ├── speech_state.go# 発言履歴・重複管理
│   ├── evaluator.go   # 評価 LLM による絞り込み
│   ├── fallback.go    # Fallback + Reason 定数
│   ├── audit.go       # SpeechAuditLog（JSONL形式）
│   ├── personality.go # パーソナリティ推論ヘルパー
│   ├── processor.go   # Processor チェーン
│   ├── validator.go   # Validator チェーン + BannedWordValidator
│   └── prompts/       # テンプレート (ja/en/question_ja/question_en)
├── memory/            # 作業メモリ (短期コンテキスト)
├── monitor/           # シグナル集約・MonitorEvent 変換
├── news/              # ニュース取得・キャッシュ
├── observer/          # Git・IDE 観察
├── persona/           # パーソナリティエンジン
├── plugin/            # プラグイン拡張
├── profile/           # 開発者プロファイル永続化
├── sensor/            # OS センサー群
├── session/           # (Legacy) セッション管理
├── transport/         # Wails / WebSocket 通知
├── types/             # 全レイヤー共通の型定義
├── weather/           # 天気取得・キャッシュ・履歴管理（history.go: 31日分の日次記録）
└── ws/                # WebSocket サーバー
docs/
└── SPEC.md            # 本仕様書
```

---

## 15. デバッグ・観測性

### 15.0 SpeechAuditLog

日次の発話ログを JSONL 形式で記録する。

- **保存場所**: `~/.sakura-kodama/audit/speech_YYYYMMDD.jsonl`
- **レコード種別**:
  - `speech`: 実際に表示されたセリフ
  - `rejected`: 直接生成でバリデーションに失敗したセリフ
  - `batch_rejected`: バッチ生成でrejectされたセリフ（理由付き）
- **レポート**: `scripts/audit_report.sh` でreject率・重複率・文字数分布を集計

**リリース基準:**
| 指標 | 目標値 |
|------|--------|
| reject率 | < 15% |
| 表示セリフ中央値 | < 40字 |
| fallback率 | < 20% |
| 1セッション内重複率 | < 10% |

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
