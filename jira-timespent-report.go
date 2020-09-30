package main

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	defaultMaxResult = 50
	hourOfDay        = 8
	dayOfMonth       = 24
	usageText                 = `Usage of jira-timespent-report:
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
        jira url (default "https://your-jira.atlassian.net")`
)

var (
	authUser      string
	authToken     string
	jiraURL       string
	jiraQuery     string
	jiraMaxResult int
	timeUnit      string
)

type issueField struct {
	Summary                       string `json:"summary"`
	Timespent                     int    `json:"timespent"`
	Timeoriginalestimate          int    `json:"timeoriginalestimate"`
	AggregateTimespent            int    `json:"aggregatetimespent"`
	AggregateTimeoriginalestimate int    `json:"aggregatetimeoriginalestimate"`
}

type issue struct {
	Id     string     `json:"id"`
	Key    string     `json:"key"`
	Fields issueField `json:"fields"`
}

type searchResult struct {
	StartAt int     `json:"startAt"`
	Total   int     `json:"total"`
	Issues  []issue `json:"issues"`
}

func init() {
	authUser = os.Getenv("AUTH_USER")
	authToken = os.Getenv("AUTH_TOKEN")
	if len(authUser) == 0 || len(authToken) == 0 {
		panic("環境変数 AUTH_USER/AUTH_TOKEN が未定義")
	}

	flag.StringVar(&jiraURL, "url", "https://your-jira.atlassian.net", "jira url")
	flag.StringVar(&jiraQuery, "query", "status = Closed", "jira query language expression")
	flag.IntVar(&jiraMaxResult, "maxresult", defaultMaxResult, "max result for pagination")
	flag.StringVar(&timeUnit, "unit", "dd", "time unit format string")
}

func main() {
	flag.Usage = func() {
		fmt.Println(usageText)
	}
	flag.Parse()
	log.Println("start")

	results := make([]searchResult, 0, 10)
	for requireNext(results) {
		result := doSearch(lastPosition(results))
		results = append(results, result)
	}

	renderCsv(results)

	log.Println("end")
}

func lastPosition(results []searchResult) int {

	if len(results) == 0 {
		return 0
	}

	return results[len(results)-1].StartAt + len(results[len(results)-1].Issues)
}

func doSearch(startAt int) searchResult {

	searchURL, err := url.Parse(jiraURL)
	if err != nil {
		log.Fatalf("url.Parse error: %v\n", err)
	}
	searchURL.Path = "/rest/api/2/search"

	searchRequest := map[string]interface{}{
		"jql": jiraQuery,
		"fields": []string{
			"summary", "timespent", "timeoriginalestimate", "aggregatetimespent", "aggregatetimeoriginalestimate",
		},
		"startAt":    startAt,
		"maxResults": jiraMaxResult,
	}
	requestBody, err := json.Marshal(searchRequest)
	if err != nil {
		log.Fatalf("json.Marshal error: %v\n", err)
	}

	req, err := http.NewRequest("POST", searchURL.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		log.Fatalf("http.NewRequest error: %v\n", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.URLEncoding.EncodeToString([]byte(authUser+":"+authToken))))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("client.Do error: %v\n", err)
	}
	defer resp.Body.Close()

	var result searchResult
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("ioutil.ReadAll error: %v\n", err)
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		log.Fatalf("decoder.Decode error: %v\n", err)
	}

	return result
}

func requireNext(results []searchResult) bool {

	if len(results) == 0 {
		return true
	}

	var numberOfIssues int

	for _, result := range results {
		numberOfIssues += len(result.Issues)
	}

	return numberOfIssues != results[0].Total
}

func renderCsv(results []searchResult) {
	log.Println(len(results))
	header := []string{
		"キー", "概要", "初期見積もり", "消費時間", "Σ初期見積もり", "Σ消費時間",
	}
	writer := csv.NewWriter(os.Stdout)
	if err := writer.Write(header); err != nil {
		log.Fatalf("writer.Write error: %v\n", err)
	}

	for _, result := range results {
		for _, issue := range result.Issues {
			record := []string{
				issue.Key,
				issue.Fields.Summary,
				convert(issue.Fields.Timeoriginalestimate),
				convert(issue.Fields.Timespent),
				convert(issue.Fields.AggregateTimeoriginalestimate),
				convert(issue.Fields.AggregateTimespent),
			}
			if err := writer.Write(record); err != nil {
				log.Fatalf("writer.Write error: %v\n", err)
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatalf("writer.Error error: %v\n", err)
	}
}

func convert(second int) string {

	var v float32
	switch strings.ToLower(timeUnit) {
	case "hh":
		v = float32(second) / 60 / 60
	case "dd":
		v = float32(second) / 60 / 60 / hourOfDay
	case "mm":
		v = float32(second) / 60 / 60 / hourOfDay / dayOfMonth
	}

	return fmt.Sprintf("%.2f", v)
}
