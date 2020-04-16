.DEFAULT_GOAL=fetchrealms

unexport GOOS
unexport GOARCH
TOPLEVEL_DIR:=$(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
BUILD_DIR:=$(TOPLEVEL_DIR)/.build
# symlinks for VS Code
LINK_DIR:=$(TOPLEVEL_DIR)/links
BUILD_FAUX:=$(BUILD_DIR)/build_faux

GO_VERSION_FAUX=$(BUILD_DIR)/go_version_$(GO_VERSION)
GO_VERSION:=1.13

include gimme.mk

FETCH_REALMS_SOURCES:=$(shell find fetchrealms -maxdepth 0 -type f -name '*.go')

$(BUILD_FAUX):
	mkdir -p $(BUILD_DIR)
	mkdir -p $(LINK_DIR)
	touch $(BUILD_FAUX)

fetchrealms: $(GO_VERSION_FAUX) $(FETCHREALMS_SOURCES)
	$(GO) build -o fetchrealms/fetchrealms $(FETCHREALMS_SOURCES)

clean:
	rm -rf $(LINK_DIR)
	rm -rf $(BUILD_DIR)
