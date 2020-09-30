# Jira Cloud で検索した Jira 課題のレポートを生成する

Jira 課題の検索結果をエクスポートするやり方だと「初期見積もり」と「消費時間」の単位がそろっていないため、自分で工数に変換するのが面倒です。

そこで、検索した Jira 課題の工数を変換するツールを作成してみました。

## 参考情報

* [Jira Cloud の REST API](https://developer.atlassian.com/server/jira/platform/rest-apis/)
* [REST API の利用例](https://developer.atlassian.com/server/jira/platform/jira-rest-api-examples/)

## 準備

Jira Cloud の REST API へアクセスするときは、リクエストに認証情報を指定します。
具体的には、 `Authorization` ヘッダーに BASIC 認証形式[^1]の値を指定します。こういう感じです。
ただし、パスワードの代わりに Atlassian Cloud で生成する API Token[^2]を使います。

```
Authorization: Basic dGVzdDp0b2tlbgo=
```

なので、ツールを実行する前に API Token を作成しておく必要があります。

[^1]: https://tools.ietf.org/html/rfc7617
[^2]: https://ja.confluence.atlassian.com/cloud/api-tokens-938839638.html

## 仕様

* Windows 10 で実行できる
* 認証情報(ユーザーIDとAPI Token)は環境変数で指定する
* 接続先 URL はコマンドライン引数で指定する
* 検索条件はコマンドライン引数で指定する
* 「初期見積もり」や「消費時間」を秒単位から変換する単位はコマンドライン引数で指定する
    - サブタスクがある Jira 課題には「Σ初期見積もり」と「Σ消費時間」の値が設定される
* 出力形式はヘッダーありの CSV

## ツールの導入


## 使い方

* 検索条件は `TIS プロジェクトでクローズした課題`
* 工数の単位は人日

```bash
$ export AUTH_USER=user@example.net
$ export AUTH_TOKEN=AKIAIOSFODNN7EXAMPLE
$ jira-timespent-report.exe --url https://your-jira.atlassian.net/ --query 'project = TIS AND status = Closed' --unit 'day'
キー,概要,初期見積もり,消費時間,Σ初期見積もり,Σ消費時間
TIS-1,sample1,1.5,1.4,0,0
TIS-2,sample2,1.0,1.2,0,0
TIS-3,sample3,2.1,2.0,2.1,2.0
```

* 検索条件は `TIS プロジェクトでクローズした課題`
* 工数の単位は人時

```bash
$ export AUTH_USER=user@example.net
$ export AUTH_TOKEN=AKIAIOSFODNN7EXAMPLE
$ jira-timespent-report.exe --url https://your-jira.atlassian.net/ --query 'project = TIS AND status = Closed' --unit 'hour'
key,summary,timeoriginalestimate,timespent,aggregatetimeoriginalestimate,aggregatetimespent
TIS-1,sample1,12.0,11.2,0,0
TIS-2,sample2,8.0,9.6,0,0
TIS-3,sample3,16.8,16.0,16.8,16.0
```
