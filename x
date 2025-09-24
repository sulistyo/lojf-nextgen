lsof -t -i:8080 | xargs -r kill -9
make build

