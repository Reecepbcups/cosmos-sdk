QMDB_DIR := $(CURDIR)/qmdb-rs
QMDB_REPO := https://github.com/LayerZero-Labs/qmdb.git
QMDB_LIB := $(QMDB_DIR)/target/release/libqmdb_ffi.a
RUST_TOOLCHAIN ?= stable

.PHONY: all clone build clean test bench

all: build

# Clone qmdb if not present
clone:
	@if [ ! -d "$(QMDB_DIR)" ]; then \
		echo "Cloning qmdb..."; \
		git clone --depth 1 $(QMDB_REPO) $(QMDB_DIR); \
	fi

# Copy FFI crate into the cloned repo and build
build: clone
	@mkdir -p $(QMDB_DIR)/qmdb-ffi/src
	@cp -r ffi/src/* $(QMDB_DIR)/qmdb-ffi/src/
	@cp ffi/Cargo.toml $(QMDB_DIR)/qmdb-ffi/Cargo.toml
	@cp ffi/qmdb_ffi.h $(QMDB_DIR)/qmdb-ffi/qmdb_ffi.h
	@grep -q 'qmdb-ffi' $(QMDB_DIR)/Cargo.toml || \
		sed -i 's/members = \[/members = [\n    "qmdb-ffi",/' $(QMDB_DIR)/Cargo.toml
	cd $(QMDB_DIR) && RUSTUP_TOOLCHAIN=$(RUST_TOOLCHAIN) cargo build -p qmdb-ffi --release
	@echo "Built $(QMDB_LIB)"

test: build
	CGO_ENABLED=1 LD_LIBRARY_PATH=$(QMDB_DIR)/target/release go test -v -count=1 ./...

bench: build
	CGO_ENABLED=1 LD_LIBRARY_PATH=$(QMDB_DIR)/target/release go test -bench=. -benchtime=2s -count=1 -timeout=600s ./...

clean:
	rm -rf $(QMDB_DIR)
