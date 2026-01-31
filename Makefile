.PHONY: build build-opt clean

BINARY_NAME=smart-proxy

build:
	go build -o $(BINARY_NAME) main.go

build-opt:
	# -s: 禁用符号表 (Omit the symbol table and debug information)
	# -w: 禁用 DWARF 生成 (Omit the DWARF symbol table)
	# 从而减小生成的可执行文件体积
	go build -ldflags="-s -w" -o $(BINARY_NAME) main.go

release: build-opt
	mkdir -p $(HOME)/.local/bin
	mv $(BINARY_NAME) $(HOME)/.local/bin/

clean:
	go clean
	rm -f $(BINARY_NAME) $(BINARY_NAME)-opt $(BINARY_NAME)-std
