#!/bin/sh
# Builds the two images the emulator needs to boot Linux:
#
#   linux/bzImage         a 32bit kernel with a serial console and no PCI,
#                         configured by linux/tiny_x86_emu.config
#   linux/initramfs.cpio  the userland, built by u-root for 386 without
#                         floating point (the emulator has no SSE)
#
# The sources are cloned next to the images. Set LINUX_SRC or UROOT_SRC to
# reuse trees which are already on the disk, and LINUX_TAG to build a
# different version of the kernel.
set -eu

tag=${LINUX_TAG:-v7.2}
out=$(cd "$(dirname "$0")/.." && pwd)/linux
src=${LINUX_SRC:-$out/src}
uroot=${UROOT_SRC:-$out/u-root}
jobs=${JOBS:-$(nproc 2>/dev/null || echo 4)}

mkdir -p "$out"

if [ ! -d "$src" ]; then
	echo "cloning Linux $tag into $src"
	git clone --depth 1 --branch "$tag" \
		https://github.com/torvalds/linux "$src"
fi

echo "configuring the kernel"
make -C "$src" ARCH=i386 tinyconfig
ARCH=i386 "$src/scripts/kconfig/merge_config.sh" -m -O "$src" \
	"$src/.config" "$out/tiny_x86_emu.config"
make -C "$src" ARCH=i386 olddefconfig

echo "building the kernel"
make -C "$src" ARCH=i386 -j"$jobs" bzImage
cp "$src/arch/x86/boot/bzImage" "$out/bzImage"

if [ ! -d "$uroot" ]; then
	echo "cloning u-root into $uroot"
	git clone --depth 1 https://github.com/u-root/u-root "$uroot"
fi

echo "building the initramfs"
# GO386=softfloat keeps SSE2 out of the binaries: the emulator implements
# only the MMX moves Go needs for its 64bit atomics.
(cd "$uroot" && GOARCH=386 GO386=softfloat CGO_ENABLED=0 \
	go run . -build=bb -o "$out/initramfs.cpio" core)

ls -l "$out/bzImage" "$out/initramfs.cpio"
