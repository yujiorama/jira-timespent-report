package jira

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	BaseURL         string
	Query           string
	Filter          string
	FieldNames      string
	MaxResult       int
	ApiVersion      string
	TimeUnit        string
	HoursPerDay     int
	DaysPerMonth    int
	Worklog         bool
	TargetYearMonth string
}

const (
	maxWorkerSize             = 10
	defaultMaxResult          = 50
	defaultHoursPerDay        = 8
	defaultDaysPerMonth       = 24
	defaultJiraRestApiVersion = "3"
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
  -targetym string
        target year month(yyyy-MM)
  -unit string
        time unit format string (default "dd")
  -url string
        jira url (default "https://your-jira.atlassian.net")
  -worklog
        collect worklog toggle`
)

var (
	config           = &Config{}
	defaultFieldText = map[string]string{
		"summary":                       "概要",
		"status":                        "ステータス",
		"timeoriginalestimate":          "初期見積もり",
		"timespent":                     "消費時間",
		"aggregatetimeoriginalestimate": "Σ初期見積もり",
		"aggregatetimespent":            "Σ消費時間",
		"started":                       "開始日時",
		"author.displayname":            "表示名",
		"author.emailaddress":           "メールアドレス",
		"timespentseconds":              "消費時間",
	}
)

func (c *Config) fields() []string {

	if c.Worklog {
		return []string{
			"started",
			"author.displayname",
			"author.emailaddress",
			"timespentseconds",
		}
	}

	return strings.Split(c.FieldNames, ",")
}

func (c *Config) checkAuthEnv() error {

	user := os.Getenv("AUTH_USER")
	token := os.Getenv("AUTH_TOKEN")

	if len(user) == 0 || len(token) == 0 {
		return fmt.Errorf("環境変数 AUTH_USER/AUTH_TOKEN が未定義")
	}

	return nil
}

func (c *Config) basicAuthorization() string {

	if err := c.checkAuthEnv(); err != nil {
		panic(err)
	}

	user := os.Getenv("AUTH_USER")
	token := os.Getenv("AUTH_TOKEN")

	return fmt.Sprintf("Basic %s", base64.URLEncoding.EncodeToString([]byte(user+":"+token)))
}

func (c *Config) dateCondition() (string, bool) {

	t, err := time.Parse("2006-01-02", config.TargetYearMonth+"-01")
	if err != nil {
		return "", false
	}

	offset := int(t.Month() - time.Now().Month())

	if config.Worklog {
		return fmt.Sprintf("worklogDate >= startOfMonth(%d) AND worklogDate <= endOfMonth(%d)", offset, offset), true
	}

	return fmt.Sprintf("updated >= startOfMonth(%d) AND updated <= endOfMonth(%d)", offset, offset), true
}

func (c *Config) FilterURL(filterID string) (*url.URL, error) {

	u, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse error: %v\nBaseURL=[%v]", err, config.BaseURL)
	}

	u.Path = fmt.Sprintf("/rest/api/%s/filter/%s", config.ApiVersion, filterID)

	return u, nil
}

func (c *Config) SearchURL() (*url.URL, error) {

	u, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse error: %v\nBaseURL=[%v]", err, config.BaseURL)
	}

	u.Path = fmt.Sprintf("/rest/api/%s/search", config.ApiVersion)

	return u, nil
}

func (c *Config) WorklogURL(key string, queryParams url.Values) (*url.URL, error) {

	u, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse error: %v\nBaseURL=[%v]", err, config.BaseURL)
	}

	u.Path = fmt.Sprintf("/rest/api/%s/issue/%s/worklog", config.ApiVersion, key)
	u.RawQuery = queryParams.Encode()

	return u, nil
}

func (c *Config) WithTimeUnit(second int) float32 {

	switch strings.ToLower(config.TimeUnit) {
	case "h", "hh":
		return float32(second) / float32(60*60)
	case "d", "dd":
		return float32(second) / float32(60*60*config.HoursPerDay)
	case "m", "mm":
		return float32(second) / float32(60*60*config.HoursPerDay*config.DaysPerMonth)
	default:
		return 0.0
	}
}

func (c *Config) TargetMonth() (*time.Time, error) {

	if len(config.TargetYearMonth) > 0 {
		t, err := time.Parse("2006-01-02", config.TargetYearMonth+"-01")
		if err != nil {
			return nil, fmt.Errorf("TargetMonth: error %v", err)
		}

		return &t, nil
	}

	t := time.Now().AddDate(0, -1, 0)
	return &t, nil
}

func (c *Config) StartedAfter() string {
	t, err := c.TargetMonth()
	if err != nil {
		return ""
	}

	startOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	return fmt.Sprintf("%d", startOfMonth.UnixNano()/int64(time.Millisecond))
}
