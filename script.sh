if ! which flyctl curl jq > /dev/null ; then
	echo "flyctl, curl, and jq must be installed to use this tool"
	exit 1
fi

while [ -z "$FLY_APP" ]; do
	if ! FLY_APP=$(flyctl status --json 2>&1 | jq -r '.Name' 2>&1); then
		echo -n "App name: "
		read FLY_APP < /dev/tty
	fi
done
export FLY_APP

while [ -z "$FLY_ORG" ]; do
	if ! FLY_ORG=$(flyctl status --json 2>&1 | jq -r '.Organization.Slug' 2>&1); then
		echo -n "Org slug (or 'personal'): "
		read FLY_ORG < /dev/tty
	fi
done

if [ -z "$FLY_API_TOKEN" ]; then
	if ! FLY_API_TOKEN=$(flyctl tokens create readonly -x5m $FLY_ORG); then
		exit 1
	fi
fi
export FLY_API_TOKEN

while [ -z "$K" ]; do
	echo -n "Number of regions: "
	read K < /dev/tty
done

# try to get list of current regions to compare against
if ! COMPARE=$(flyctl m list --json | jq -r 'map(.region) | join(",")' 2>&1); then
	unset COMPARE
fi

PROM_URL="https://api.fly.io/prometheus/$FLY_ORG/api/v1/query"
QUERY='query=sum(increase(fly_edge_http_responses_count{app="'$FLY_APP'"}[24h])) by (region)'
AUTH="Authorization: $FLY_API_TOKEN"
BR_URL="https://best-regions.fly.dev?k=$K&compare=$COMPARE"

curl "$PROM_URL" -s --data-urlencode "$QUERY" -H "$AUTH" \
| curl -s "$BR_URL" -XPOST --data-binary @-
