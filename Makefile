build:
#   Uncomment below if you want to use go-dep
#	dep ensure
	env GOOS=linux go build -ldflags="-s -w" -o bin/hugotos3 hugotos3/main.go