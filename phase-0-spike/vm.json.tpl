{
  "boot-source": {
    "kernel_image_path": "__KERNEL__",
    "boot_args": "console=ttyS0 reboot=k panic=1 pci=off rw"
  },
  "drives": [
    {
      "drive_id": "rootfs",
      "path_on_host": "__ROOTFS__",
      "is_root_device": true,
      "is_read_only": false
    }
  ],
  "network-interfaces": [
    {
      "iface_id": "eth0",
      "guest_mac": "AA:BB:CC:00:00:02",
      "host_dev_name": "tap-fc0"
    }
  ],
  "machine-config": {
    "vcpu_count": 2,
    "mem_size_mib": 2048,
    "smt": false
  }
}
