package jira

type Status struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type IssueField struct {
	Summary                       string `json:"summary"`
	Timespent                     int    `json:"timespent"`
	Timeoriginalestimate          int    `json:"timeoriginalestimate"`
	Aggregatetimespent            int    `json:"aggregatetimespent"`
	Aggregatetimeoriginalestimate int    `json:"aggregatetimeoriginalestimate"`
	Status                        Status `json:"status,omitempty"`
}

type Issue struct {
	Id     string     `json:"id"`
	Key    string     `json:"key"`
	Fields IssueField `json:"fields"`
}

type Issues []Issue

type IssueSearchResult struct {
	StartAt    int    `json:"startAt"`
	Total      int    `json:"total"`
	MaxResults int    `json:"maxResults"`
	Issues     Issues `json:"issues"`
}

type IssueSearchResults []IssueSearchResult

type WorklogField struct {
	Key    string
	Author struct {
		Displayname  string `json:"displayName"`
		Emailaddress string `json:"emailAddress"`
	} `json:"author"`
	Started          string `json:"started"`
	Timespentseconds int    `json:"timespentSeconds"`
}

type Worklogs []WorklogField

type WorklogResult struct {
	StartAt    int      `json:"startAt"`
	Total      int      `json:"total"`
	MaxResults int      `json:"maxResults"`
	Worklogs   Worklogs `json:"worklogs"`
}

type WorklogResults []WorklogResult
