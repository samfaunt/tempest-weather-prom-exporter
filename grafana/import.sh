#!/bin/sh
# Import the Tempest dashboard through Grafana's API. Unlike the web import
# page, the API substitutes the station serial into every query, which keeps
# the dashboard free of variables so it can be shared externally.
#
# Usage: GRAFANA_URL=https://grafana.example.com GRAFANA_TOKEN=glsa_... \
#          grafana/import.sh <prometheus-datasource-name> [station-serial]
#
# GRAFANA_TOKEN is a service account token with the Editor role. The serial
# defaults to .+ (all stations).
set -eu

for cmd in curl jq; do
	command -v "$cmd" >/dev/null || {
		echo "$cmd is required" >&2
		exit 2
	}
done

if [ $# -lt 1 ] || [ $# -gt 2 ] || [ -z "${GRAFANA_URL:-}" ] || [ -z "${GRAFANA_TOKEN:-}" ]; then
	sed -n '6,10p' "$0" | sed 's/^# \{0,1\}//' >&2
	exit 2
fi
datasource=$1
serial=${2:-.+}
case $serial in
*\"* | *\\*)
	echo "station serial must not contain quotes or backslashes: $serial" >&2
	exit 2
	;;
esac

url=${GRAFANA_URL%/}
auth="Authorization: Bearer $GRAFANA_TOKEN"
dashboard=$(dirname "$0")/tempest-dashboard.json

# Each curl runs on its own (not piped into jq) so set -e stops on errors.
name=$(jq -rn --arg s "$datasource" '$s | @uri')
datasource_json=$(curl -fsS -H "$auth" "$url/api/datasources/name/$name")
ds_uid=$(printf '%s' "$datasource_json" | jq -r .uid)

body=$(jq --arg ds "$ds_uid" --arg serial "$serial" '{
	dashboard: .,
	overwrite: true,
	inputs: [
		{name: "DS_PROMETHEUS", type: "datasource", pluginId: "prometheus", value: $ds},
		{name: "VAR_SERIAL", type: "constant", value: $serial}
	]
}' "$dashboard")
result=$(printf '%s' "$body" | curl -fsS -X POST -H "$auth" -H 'Content-Type: application/json' \
	--data-binary @- "$url/api/dashboards/import")
printf '%s' "$result" | jq -r --arg url "$url" '"Imported: \($url)\(.importedUrl)"'
