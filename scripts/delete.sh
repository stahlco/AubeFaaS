#!/bin/bash

# delete.sh function-name

set -e

if ! command -v curl &> /dev/null
then
  echo "curl could not be found but is a prerequiste for this script"
  exit
fi

curl http://localhost:8090/delete --data "{\"name\": \"$1\"}"