include $(TOPDIR)/rules.mk

PKG_VERSION:=$(or $(ROUTEGUARD_VERSION),0.0.0)
PKG_RELEASE:=1

LUCI_TITLE:=LuCI for RouteGuard
LUCI_DEPENDS:=+routeguard +luci-base
LUCI_PKGARCH:=all
PKG_LICENSE:=GPL-2.0-only

include $(TOPDIR)/feeds/luci/luci.mk

# call BuildPackage - OpenWrt buildroot signature
