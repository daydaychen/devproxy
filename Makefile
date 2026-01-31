.PHONY: build build-opt clean

BINARY_NAME=smart-proxy
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-X smart-proxy/pkg/util.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) main.go

build-opt:
	# -s: 禁用符号表, -w: 禁用 DWARF 生成, 减小体积
	go build -ldflags="-s -w $(LDFLAGS)" -o $(BINARY_NAME) main.go

release: build-opt
	mkdir -p $(HOME)/.local/bin
	mv $(BINARY_NAME) $(HOME)/.local/bin/

clean:
	go clean
	rm -f $(BINARY_NAME) $(BINARY_NAME)-opt $(BINARY_NAME)-std
