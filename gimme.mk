GOROOT:=$(shell gimme $(GO_VERSION) | grep GOROOT | sed "s/.*'\(.*\)'.*/\1/")
GO:=GOROOT=$(GOROOT) $(GOROOT)/bin/go
GO_BIN:=$(LINK_DIR)/go
GOROOT_LINK:=$(LINK_DIR)/goroot

$(GO_VERSION_FAUX): $(BUILD_FAUX)
	gimme $(GO_VERSION)
	ln -s -T $(GOROOT)/bin/go $(GO_BIN)
	ln -s $(GOROOT) $(GOROOT_LINK)
	touch $(GO_VERSION_FAUX)