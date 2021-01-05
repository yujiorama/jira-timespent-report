package jira

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
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
	clock           func() time.Time
}

const (
	maxWorkerSize             = 10
	defaultMaxResult          = 50
	defaultHoursPerDay        = 8
	defaultDaysPerMonth       = 24
	defaultJiraRestApiVersion = "3"
	usageText                 = `Usage of jira-timespent-report (v%s):
  $ jira-timespent-report [options]

Example:
  # get csv report by cli
  $ AUTH_USER=yyyy AUTH_TOKEN=aaaabbbb jira-timespent-report -url https://your-jira.atlassian.net -maxresult 10 -unit dd -query "status = Closed" -targetym 2020-08

  # get csv report by http server
  $ AUTH_USER=yyyy AUTH_TOKEN=aaaabbbb jira-timespent-report -server &
  $ curl localhost:8080/?url=https://your-jira.atlassian.net&maxresult=10&unit=dd&query=status+%%3DClosed&targetym=2020-08

Options:
`
)

var (
	config           = &Config{clock: time.Now}
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

func (c *Config) SetQueryParams(queryParams url.Values) {

	for key, vs := range queryParams {
		if len(vs) == 0 {
			continue
		}
		value := vs[0]

		switch strings.ToLower(key) {
		case "baseurl":
			c.BaseURL = value
		case "query":
			c.Query = value
		case "filter":
			c.Filter = value
		case "fieldnames":
			c.FieldNames = value
		case "maxresult":
			i, _ := strconv.Atoi(value)
			c.MaxResult = i
		case "apiversion":
			c.ApiVersion = value
		case "timeunit":
			c.TimeUnit = value
		case "hoursperday":
			i, _ := strconv.Atoi(value)
			c.HoursPerDay = i
		case "dayspermonth":
			i, _ := strconv.Atoi(value)
			c.DaysPerMonth = i
		case "worklog":
			b, _ := strconv.ParseBool(value)
			c.Worklog = b
		case "targetyearmonth":
			c.TargetYearMonth = value
		}
	}
}

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

	targetTime, err := time.Parse("2006-01-02", c.TargetYearMonth+"-01")
	if err != nil {
		return "", false
	}
	currentTime := c.clock()

	if targetTime.Year() > currentTime.Year() {
		return "", false
	}

	offset := 0
	if targetTime.Year() == currentTime.Year() {
		if targetTime.Month() > currentTime.Month() {
			return "", false
		}
		offset = int(targetTime.Month() - currentTime.Month())

	} else {
		monthDiff := 12 - int(targetTime.Month()) + int(currentTime.Month())
		yearDiff := (currentTime.Year() - targetTime.Year() - 1) * 12
		offset = -monthDiff - yearDiff
	}

	if c.Worklog {
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
