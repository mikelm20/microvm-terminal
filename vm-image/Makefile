# VM image build. Run on the Linux x86_64 host (learn-01), not on macOS/arm64.
#
#   sudo make all            build both rootfs and kernel into $OUT_DIR
#   sudo make rootfs         build/refresh just the rootfs
#   sudo make kernel         download the kernel (only runs if missing)
#   sudo make clean          remove $OUT_DIR entirely
#
# Default $OUT_DIR lives on the dedicated NVMe so large artifacts don't touch /.

OUT_DIR ?= /var/lib/firecracker/images
ROOTFS_SIZE_MB ?= 4096
KERNEL_VERSION ?= 6.1.102

# sudo drops /usr/local/go/bin from PATH; add it back so the nested make for
# the guest-agent can find the go toolchain.
export PATH := /usr/local/go/bin:$(PATH)

.PHONY: all rootfs kernel guest-agent clean

all: rootfs kernel

# Build the guest-agent and stage it in this directory so the Dockerfile's
# COPY can pick it up. The binary is bundled into the rootfs.
guest-agent:
	$(MAKE) -C $(CURDIR)/../guest-agent build
	cp $(CURDIR)/../guest-agent/bin/learn-guest-agent $(CURDIR)/learn-guest-agent

rootfs: guest-agent
	OUT_DIR=$(OUT_DIR) ROOTFS_SIZE_MB=$(ROOTFS_SIZE_MB) bash $(CURDIR)/build-rootfs.sh

kernel:
	OUT_DIR=$(OUT_DIR) KERNEL_VERSION=$(KERNEL_VERSION) bash $(CURDIR)/fetch-kernel.sh

clean:
	rm -rf $(OUT_DIR)
