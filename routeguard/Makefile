include $(TOPDIR)/rules.mk

PKG_NAME:=routeguard
PKG_VERSION:=$(or $(ROUTEGUARD_VERSION),0.0.0)
PKG_RELEASE:=1
PKG_LICENSE:=GPL-2.0-only

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/routeguard
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Selective WAN kill switch
  DEPENDS:=$(GO_ARCH_DEPENDS) +firewall4 +ca-bundle
endef

define Package/routeguard/conffiles
/etc/config/routeguard
endef

define Build/Prepare
	$(INSTALL_DIR) $(PKG_BUILD_DIR)
	$(CP) ./src/. $(PKG_BUILD_DIR)/
endef

define Build/Configure
endef

define Build/Compile
	cd $(PKG_BUILD_DIR) && $(GO_PKG_TARGET_VARS) $(GO_PKG_BUILD_VARS) \
		CGO_ENABLED=0 GOFLAGS= GOPROXY=off GOSUMDB=off \
		go build -trimpath -buildvcs=false -ldflags="-s -w" -o routeguard ./cmd/routeguard
endef

define Package/routeguard/install
	$(INSTALL_DIR) $(1)/etc/config $(1)/etc/init.d $(1)/usr/sbin $(1)/lib/upgrade/keep.d $(1)/etc/hotplug.d/iface
	$(INSTALL_CONF) ./files/routeguard.config $(1)/etc/config/routeguard
	$(INSTALL_BIN) ./files/routeguard.init $(1)/etc/init.d/routeguard
	$(INSTALL_DATA) ./files/routeguard.keep $(1)/lib/upgrade/keep.d/routeguard
	$(INSTALL_BIN) ./files/routeguard.hotplug $(1)/etc/hotplug.d/iface/95-routeguard
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/routeguard $(1)/usr/sbin/routeguard
endef

$(eval $(call BuildPackage,routeguard))
