# DESIGN.md \- 職場用パーソナルGTDツール 設計書

## 1\. コンセプトと設計思想

GTD（Getting Things Done）の本来の目的である\*\*「頭の中を空にして、次にやるべき具体的な行動（Next Action）を迷わず選ぶこと」\*\*に特化した個人用ツール。

### コア原則

1. **すべてを「Item」として統合する**: 「Inbox（未整理）」「タスク（Next Action）」「メモ（Reference）」を別テーブルに分けず、単一の `items` テーブルの `status` カラムで管理する。これにより、「最初はメモだったものがタスクになり、やがて資料として保管される」という状態変化をシームレスに扱える。  
2. **加工はOSに任せ、ツールは「貼るだけ」にする**: 以前のClip機能の反省を活かし、ツール内での画像書き込みやCSVグラフ化は行わない。WindowsのSnipping Tool等で加工したスクショを `Ctrl + V` で即座に貼り付けられる軽快さに特化する。  
3. **週次レビューをシステム化する**: GTDが形骸化しないよう、「止まっているプロジェクト（Next Actionが0件のプロジェクト）」を自動抽出する3ステップのレビュー画面を組み込む。
4. Materialデザインに準拠すること
5. UIに絵文字は使わない（SVGアイコン等を利用する）

---

## 2\. データベース設計 (SQLite)

\-- 1\. プロジェクト（複数の行動を要するゴール）

CREATE TABLE projects (

    id INTEGER PRIMARY KEY AUTOINCREMENT,

    name TEXT NOT NULL,

    description TEXT,

    status TEXT NOT NULL DEFAULT 'active', \-- 'active', 'someday', 'done'

    last\_reviewed\_at DATETIME,

    created\_at DATETIME DEFAULT CURRENT\_TIMESTAMP,

    updated\_at DATETIME DEFAULT CURRENT\_TIMESTAMP

);

\-- 2\. アイテム（Inbox / タスク / メモ の統合モデル）

CREATE TABLE items (

    id INTEGER PRIMARY KEY AUTOINCREMENT,

    project\_id INTEGER REFERENCES projects(id) ON DELETE SET NULL,

    title TEXT NOT NULL,

    body TEXT, \-- 詳細メモ（プレーンテキスト / 簡易Markdown）

    status TEXT NOT NULL DEFAULT 'inbox', 

    \-- status: 'inbox'(未整理), 'next'(次にやる), 'waiting'(待ち), 'someday'(いつか), 'reference'(資料・メモ), 'done'(完了)

    due\_date DATE, \-- 厳密な締切があるもののみ設定

    last\_reviewed\_at DATETIME,

    created\_at DATETIME DEFAULT CURRENT\_TIMESTAMP,

    updated\_at DATETIME DEFAULT CURRENT\_TIMESTAMP

);

\-- 3\. コンテキスト（場所・状況・相手のタグ）

CREATE TABLE contexts (

    id INTEGER PRIMARY KEY AUTOINCREMENT,

    name TEXT NOT NULL UNIQUE \-- 例: '@PC', '@会議', '@Aさん', '@5分'

);

\-- 4\. アイテムとコンテキストの中間テーブル

CREATE TABLE item\_contexts (

    item\_id INTEGER REFERENCES items(id) ON DELETE CASCADE,

    context\_id INTEGER REFERENCES contexts(id) ON DELETE CASCADE,

    PRIMARY KEY (item\_id, context\_id)

);

\-- 5\. 添付・関連リンク（URL / ローカルパス / スクショ画像の統合管理）

CREATE TABLE attachments (

    id INTEGER PRIMARY KEY AUTOINCREMENT,

    item\_id INTEGER REFERENCES items(id) ON DELETE CASCADE,

    label TEXT,          \-- 表示名（例: '仕様書Excel', 'エラー画面スクショ'）

    target TEXT NOT NULL, \-- URL文字列、ファイルパス(C:\\..., \\\\server\\...)、または保存画像ファイル名

    type TEXT NOT NULL,   \-- 'url', 'path', 'image'

    created\_at DATETIME DEFAULT CURRENT\_TIMESTAMP

);

---

## 3\. 画面レイアウト（1画面完結・3ペイン構成）

画面遷移（フルリロード）を行わず、htmxによる部分書き換えで動作する構成。

### 3.1 上部バー：Quick Capture（即時投入）

- 画面上部に常時フォーカス可能な1行テキスト入力欄を配置（ショートカットキー `Alt + N` や `/` 等でいつでもフォーカス可能にするとなお良い）。  
- 入力して `Enter` を押すと `hx-post="/items"` が実行され、`status = 'inbox'` として即座に登録される。

### 3.2 左ペイン：ナビゲーション＆フィルタ

- **Inbox**（未処理件数バッジ付き）  
- **Next Actions**（今やるべき行動一覧。下部で `@PC` などのContextタグによる絞り込みが可能）  
- **Waiting For**（他人の返信・外部要因待ち一覧）  
- **Projects**（アクティブなプロジェクト一覧）  
- **Someday**（保留リスト）  
- **References**（行動を伴わない資料・メモ一覧）  
- **Weekly Review**（週次レビュー開始ボタン）

### 3.3 中央ペイン：メインリスト（またはレビュー画面）

- 左ペインで選択したビューのアイテム一覧を表示。  
- 各行にはチェックボックス（クリックで `hx-patch="/items/{id}/status"` により `done` へ変更しフェードアウト）、タイトル、所属Projectバッジ、Contextタグ、添付アイコンを表示。  
- 行をクリックすると `hx-get="/items/{id}/detail"` が走り、右ペインに詳細が表示される。

### 3.4 右ペイン：インスペクタ（詳細編集・GTD仕分け・Clip貼り付け）

- **ステータス変更ボタン群**: `Inbox` / `Next` / `Waiting` / `Someday` / `Reference` / `Done` をワンクリックで切り替え。  
- **Project選択プルダウン** ＆ **Contextタグ選択**: `hx-put` \+ `hx-trigger="change"` で変更時に自動保存。  
- **詳細メモ (`body`)**: テキストエリア（フォーカスアウト時や一定時間入力停止時に自動保存）。  
- **Attachments（関連ファイル・URL・スクショ）**:  
  - URLや社内ファイルパス（`\\server\...`）の追加フォーム。  
  - `type = 'path'` の項目には **「開く」ボタン**（`hx-post="/attachments/{id}/open"` でGoサーバー側がエクスプローラーや関連付けアプリを起動）と **「パスをコピー」ボタン** を配置。  
  - **スクリーンショット貼り付け**: 詳細ペイン表示中に `Ctrl + V` を押すと、クリップボード内の画像データを検知して `POST /items/{id}/paste-image` へ自動送信。`./data/images/` に保存し、サムネイル一覧に即座に追加する。

---

## 4\. 週次レビュー機能（3ステップ・ウィザード）

GTDの要である週次レビューを迷わず完了させるための専用フロー。

### Step 1: Get Clear（Inboxを空にする）

- **抽出条件**: `SELECT * FROM items WHERE status = 'inbox' ORDER BY created_at ASC`  
- **アクション**: 1件ずつ（またはリストで）表示し、その場で `Next Action` / `Waiting` / `Someday` / `Reference` / `Done` / `Delete` のいずれかに振り分ける。残り0件になったらStep 2へ進む。

### Step 2: Get Current（止まっているプロジェクトの点検）

- **抽出条件**: `status = 'active'` の全プロジェクトを取得し、特に\*\*「紐づく `status = 'next'` のアイテムが0件のプロジェクト」\*\*を最上部に警告表示する。  
    
  SELECT p.\*, COUNT(i.id) AS next\_action\_count  
    
  FROM projects p  
    
  LEFT JOIN items i ON p.id \= i.project\_id AND i.status \= 'next'  
    
  WHERE p.status \= 'active'  
    
  GROUP BY p.id  
    
  ORDER BY next\_action\_count ASC, p.id ASC;  
    
- **アクション**: `next_action_count = 0` のプロジェクトに対し、その場で「次にやる具体的な行動（Item）」を1つ追加するか、プロジェクト自体を `done` / `someday` に変更する。確認したプロジェクトは `last_reviewed_at` を現在日時に更新する。

### Step 3: Get Creative（Waiting / Somedayの棚卸し）

- **抽出条件**: `status IN ('waiting', 'someday')` のうち、`last_reviewed_at`（未設定なら `updated_at`）が7日以上前のアイテム。  
- **アクション**:  
  - 「今週やる」→ `status = 'next'` に変更  
  - 「引き続き待つ/保留」→ `last_reviewed_at = CURRENT_TIMESTAMP` に更新してリストから消す  
  - 「もう不要」→ 削除または `done`

---

## 5\. 主要機能の実装仕様メモ

### 5.1 Launcher: ローカルパス起動（Windows/Mac）

ブラウザのセキュリティ制約（`file://` リンクのブロック）を回避するため、ローカルで稼働するGoサーバーがOSコマンドを実行してファイルやフォルダを開く。

- エンドポイント: `POST /attachments/{id}/open`  
- Go側の処理イメージ:  
    
  // 例: Windows環境でパスを既定のアプリまたはエクスプローラーで開く  
    
  cmd := exec.Command("cmd", "/c", "start", "", targetPath)  
    
  err := cmd.Start()

### 5.2 Ctrl \+ V によるスクショ即時保存（軽量Clip機能）

フロントエンドのビルドを行わず、`static/app.js` に20行程度のイベントリスナーを記述する。

document.addEventListener('paste', async (e) \=\> {

  const detailPane \= document.getElementById('item-detail-pane');

  if (\!detailPane \|\| \!detailPane.dataset.itemId) return;

  const items \= e.clipboardData?.items;

  if (\!items) return;

  for (const item of items) {

    if (item.type.indexOf('image') \!== \-1) {

      const file \= item.getAsFile();

      const formData \= new FormData();

      formData.append('image', file);

      const itemId \= detailPane.dataset.itemId;

      await fetch(\`/items/\${itemId}/paste-image\`, {

        method: 'POST',

        body: formData

      });

      // htmxで添付一覧部分をリロード発火

      htmx.trigger('\#attachments-list', 'reload-attachments');

    }

  }

});