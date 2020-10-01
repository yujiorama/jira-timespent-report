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
* フィールド名はコマンドライン引数で指定する
* 「初期見積もり」や「消費時間」を秒単位から変換する単位はコマンドライン引数で指定する
    - サブタスクがある Jira 課題には「Σ初期見積もり」と「Σ消費時間」の値が設定される
* 出力形式はヘッダーありの CSV

## ツールの導入

```bash
go get bitbucket.org/yujiorama/jira-timespent-report
```

## 使い方

```bash
$ jira-timespent-report -h
Usage of jira-timespent-report:
  -api string
        number of API Version of Jira REST API (default "3")
  -days int
        work days per month (default 24)
  -fields string
        fields of jira issue (default "summary,status,timespent,timeoriginalestimate,aggregatetimespent,aggregatetimeoriginalestimate")
  -filter string
        jira search filter id
  -hours int
        work hours per day (default 8)
  -maxresult int
        max result for pagination (default 50)
  -query string
        jira query language expression (default "status = Closed AND updated >= startOfMonth(-1) AND updated <= endOfMonth(-1)")
  -unit string
        time unit format string (default "dd")
  -url string
        jira url (default "https://your-jira.atlassian.net")
```

## ライセンス

[MIT](./LICENSE)
