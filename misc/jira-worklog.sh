#!/bin/bash
set -xu

mkdir -p "${PWD}/.tmp"
WORKDIR=$(mktemp -d -p "${PWD}/.tmp" XXXXX)

trap "rm -rf ${WORKDIR}" 1 2 3 15

export BASE_URL="https://your-jira.atlassian.net"
export SEARCH_URL="${BASE_URL}/rest/api/3/search"
export ACCEPT_JSON="Accept: application/json"
export AUTHORIZATION="${AUTHORIZATION:-user:token}"
export PROJECT="${PROJECT:-TIS}"

NOW=$(date "+%Y%m%d-%H%M%S")
CURRENT_YM=$(date +"%Y%m")
LAST_YM=$(date +"%Y%m" --date="1 month ago")
TARGET_YM="${1:-${LAST_YM}}"

if [[ ! "${TARGET_YM}" =~ ^[0-9]{4}-[0-9]{2}$ ]]; then
  echo "書式エラー: ${TARGET_YM}"
  echo "引数の書式は YYYY-MM"
  exit 1
fi

MONTH_OFFSET=$(( TARGET_YM - CURRENT_YM ))
JQL=$(jq -sRr @uri <<< "project IN (${PROJECT}) AND worklogDate >= startOfMonth(${MONTH_OFFSET}) AND worklogDate <= endOfMonth(${MONTH_OFFSET}) ORDER BY KEY ASC")

COUNT=$(curl --silent \
             --user "${AUTHORIZATION}" \
             --header "${ACCEPT_JSON}" \
             "${SEARCH_URL}?jql=${JQL}&expand=changelog&startAt=0&maxresult=1" | \
        jq -r '.total' )

MAXRESULT=50
if [[ ${COUNT} -gt 500 ]]; then
  MAXRESULT=300
fi

PAGE=$(( COUNT / MAXRESULT + 1))

for ((i=0; i < ${PAGE}; i++))
do
  OFFSET=$(( i * 50))
  curl --silent \
       --user "${AUTHORIZATION}" \
       --header "${ACCEPT_JSON}" \
       "${SEARCH_URL}?jql=${JQL}&expand=changelog&startAt=${OFFSET}&maxresult=${MAXRESULT}" | \
  jq -r '.issues[].key' > "${WORKDIR}/idlist.${i}"
  if command -v dos2unix >/dev/null 2>&1; then dos2unix --quiet "${WORKDIR}/idlist.${i}"; fi
done

find "${WORKDIR}" -type f -name idlist.\* -exec cat {} \; | \
sort -d -u | \
tee "${WORKDIR}/idlist"

STARTYM="$(date +"%Y-%m" --date="${MONTH_OFFSET} month")"
STARTEDAFTER="$(date +%s --date="${STARTYM}-01T00:00:00+0900")000"
while IFS= read -r LINE
do
  export KEY=$LINE
  curl --silent \
       --user "${AUTHORIZATION}" \
       --header "${ACCEPT_JSON}" \
       "${BASE_URL}/rest/api/3/issue/${KEY}/worklog?startedAfter=${STARTEDAFTER}" | \
  jq -r ".worklogs[] | \
         [ env.KEY, .started, .author.displayName, .author.accountId, .author.emailAddress, .timeSpentSeconds] | \
         @csv" > "${WORKDIR}/out.${KEY}"
  if command -v dos2unix >/dev/null 2>&1; then dos2unix --quiet "${WORKDIR}/out.${KEY}"; fi
done < "${WORKDIR}/idlist"

( \
  echo -en '\xef\xbb\xbf';
  echo "key,started,displayName,accountId,mailAddress,timeSpentSeconds";
  find "${WORKDIR}" -type f -name out.\* -exec cat {} \; | \
  awk -v FS=, -v OFS=, -v startym="${STARTYM}" '{v=substr($2,2,7);if(v==startym){print}}';
) | sort -d -k 1,1 -k 1,2 > "output-${NOW}.csv"

rm -rf "${WORKDIR}"
