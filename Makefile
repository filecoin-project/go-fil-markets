all: build
.PHONY: all

SUBMODULES=

FFI_PATH:=./extern/filecoin-ffi/
FFI_DEPS:=libfilecoin.a filecoin.pc filecoin.h
FFI_DEPS:=$(addprefix $(FFI_PATH),$(FFI_DEPS))

$(FFI_DEPS): .filecoin-build ;

.filecoin-build: $(FFI_PATH)
	$(MAKE) -C $(FFI_PATH) $(FFI_DEPS:$(FFI_PATH)%=%)
	@touch $@

.update-modules:
	git submodule update --init --recursive
	@touch $@

pieceio: .update-modules .filecoin-build
	go build ./pieceio
.PHONY: pieceio
SUBMODULES+=pieceio

filestore:
	go build ./filestore
.PHONY: filestore
SUBMODULES+=filestore

build: $(SUBMODULES)

clean:
	rm -f .filecoin-build
	rm -f .update-modules