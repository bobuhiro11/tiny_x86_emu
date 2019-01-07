#/bin/bash
for bin in $(ls ./xv6-public/xv6.img)
do
  tmp_file=$(mktemp)
  qemu-system-i386 -hdb ${bin} -S -gdb tcp::1234 -nographic 2>/dev/null &
  qemu_pid=$!
  gdb -x ./gdb.script 2>/dev/null > $tmp_file
  echo bin=${bin} qemu_pid=${qemu_pid} >&2

  cat $tmp_file | grep -e "eax\s*0x" \
             -e "ecx\s*0x" \
             -e "edx\s*0x" \
             -e "ebx\s*0x" \
             -e "esp\s*0x" \
             -e "ebp\s*0x" \
             -e "esi\s*0x" \
             -e "edi\s*0x" \
             -e "eip\s*0x" \
             -e "eflags\s*0x" \
             -e "cs\s*0x" \
             -e "ss\s*0x" \
             -e "ds\s*0x" \
             -e "es\s*0x" \
             -e "fs\s*0x" \
             -e "gs\s*0x" \
             | awk '{ if ($1=="eax") print "- " $1 ": " $2; else print "  " $1 ": " $2; }'
  kill ${qemu_pid};
done
