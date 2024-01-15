OS_VALUES := darwin linux windows
ARCH_VALUES := 386 amd64 arm64
OS_ARCH_COMBINATIONS := $(filter-out darwin_386, $(foreach os, $(OS_VALUES), $(foreach arch, $(ARCH_VALUES), $(os)_$(arch))))
TAG ?= latest

all: $(OS_ARCH_COMBINATIONS) tar_archives checksums

$(OS_ARCH_COMBINATIONS):
	@echo "Building binary for $(subst _, ,$@)..."
	@GOOS=$(word 1, $(subst _, ,$@)) GOARCH=$(word 2, $(subst _, ,$@)) go build -ldflags "-w -s" -o bin/fbg_scrape_$(TAG)_$@ .

tar_archives: $(OS_ARCH_COMBINATIONS)
	@echo "Creating tar.gz archives..."
	@for target in $(OS_ARCH_COMBINATIONS); do \
		tar -czvf bin/fbg_scrape_$(TAG)_$$target.tar.gz -C bin fbg_scrape_$(TAG)_$$target; \
	done

checksums:
	@echo "Creating checksums..."
	@sha256sum bin/fbg_scrape_$(TAG)_* > bin/fbg_scrape_$(TAG)_checksums.txt

clean:
	@rm -rf bin

run: build
	@./bin/scrape

build:
	@go build -o bin/scrape .

.PHONY: all clean run build $(OS_ARCH_COMBINATIONS)
