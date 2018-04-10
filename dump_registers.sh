#/bin/bash
for bin in $(ls ./xv6-public/xv6.img)
# for bin in $(ls ./guest/mbr.bin)
# for bin in $(ls ./guest/*.bin)
do
  # reg_file=$(echo $bin | sed -e 's/.bin/.regs/')
  reg_file=$(echo $bin | sed -e 's/.img/.regs/')
  qemu-system-i386 -hdb ${bin} -S -gdb tcp::1234 -nographic 2>/dev/null &
  qemu_pid=$!;
  echo bin=${bin} reg_file=${reg_file} qemu_pid=${qemu_pid};
  gdb -x ./gdb.script 2>/dev/null \
      | grep -e "eax\s*0x" \
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
             -e "===" \
     | awk '{print $1,$2;}' > ${reg_file}
  gdb_pid=$!;
  sleep 10
  kill ${qemu_pid};
  kill ${gdb_pid};
done
