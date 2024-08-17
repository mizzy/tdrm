.PHONY: clean install dist

tdrm: go.* *.go cmd/tdrm/main.go
	go build -o $@ cmd/tdrm/main.go

clean:
	rm -rf tdrm dist/

install:
	go install github.com/mizzy/tdrm/cmd/tdrm

dist:
	goreleaser build --snapshot --clean
