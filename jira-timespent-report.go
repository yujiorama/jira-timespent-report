package main

import (
	"bitbucket.org/yujiorama/jira-timespent-report/cmd/cli"
	"bitbucket.org/yujiorama/jira-timespent-report/cmd/web"
	"bitbucket.org/yujiorama/jira-timespent-report/jira"
	"os"
)

func main() {
	jira.SetFlags()

	if web.CanDo() {
		web.Do()
		os.Exit(0)
	}

	cli.Do()
	os.Exit(0)
}
