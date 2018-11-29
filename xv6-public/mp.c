// Multiprocessor support
// Search memory for MP description structures.
// http://developer.intel.com/design/pentium/datashts/24201606.pdf

#include "types.h"
#include "defs.h"
#include "param.h"
#include "memlayout.h"
#include "mp.h"
#include "x86.h"
#include "mmu.h"
#include "proc.h"

struct cpu cpus[NCPU];
int ncpu;
uchar ioapicid;

static uchar
sum(uchar *addr, int len)
{
  int i, sum;

  sum = 0;
  for(i=0; i<len; i++)
    sum += addr[i];
  return sum;
}

// Look for an MP structure in the len bytes at addr.
static struct mp*
mpsearch1(uint a, int len)
{
  uchar *e, *p, *addr;

  addr = P2V(a);
  e = addr+len;
  // cprintf("addr=%p e=%p\n", addr, e);
  for(p = addr; p < e; p += sizeof(struct mp)) {
    // cprintf("The addr of struct mp=0x%p(0x%p) memcmp=%p p[0]=%p p[1]=%p p[2]=%p p[3]=%p sum=%d\n",
    //         p,V2P(p),
    //         memcmp(p, "_MP_", 4),
    //         p[0],p[1],p[2],p[3],
    //         sum(p, sizeof(struct mp)));
    if(memcmp(p, "_MP_", 4) == 0 && sum(p, sizeof(struct mp)) == 0)
      return (struct mp*)p;
  }
  // p = addr;
  // if(memcmp(p, "_MP_", 4) == 0 && sum(p, sizeof(struct mp)) == 0)
  //   return (struct mp*)p;
  return 0;
}

// Search for the MP Floating Pointer Structure, which according to the
// spec is in one of the following three locations:
// 1) in the first KB of the EBDA;
// 2) in the last KB of system base memory;
// 3) in the BIOS ROM between 0xE0000 and 0xFFFFF.
static struct mp*
mpsearch(void)
{
  uchar *bda;
  uint p;
  struct mp *mp;

  bda = (uchar *) P2V(0x400);
  if((p = ((bda[0x0F]<<8)| bda[0x0E]) << 4)){
    // if((mp = mpsearch1(p, 1024)))
    if((mp = mpsearch1(p, 128)))
      return mp;
  } else {
    p = ((bda[0x14]<<8)|bda[0x13])*1024;
    if((mp = mpsearch1(p-1024, 1024)))
      return mp;
  }
  return mpsearch1(0xF0000, 0x10000);
}

// Search for an MP configuration table.  For now,
// don't accept the default configurations (physaddr == 0).
// Check for correct signature, calculate the checksum and,
// if correct, check the version.
// To do: check extended table checksum.
static struct mpconf*
mpconfig(struct mp **pmp)
{
  struct mpconf *conf;
  struct mp *mp;

  if((mp = mpsearch()) == 0 || mp->physaddr == 0) {
      cprintf("at mp.c L.79 mp=%p ", mp);
      return 0;
  }
  conf = (struct mpconf*) P2V((uint) mp->physaddr);
  if(memcmp(conf, "PCMP", 4) != 0) {
      panic("at mp.c L.83");
      return 0;
  }
  if(conf->version != 1 && conf->version != 4) {
      panic("at mp.c L.86");
      return 0;
  }
  if(sum((uchar*)conf, conf->length) != 0) {
      panic("at mp.c L.89");
      return 0;
  }
  *pmp = mp;
  return conf;
}

void
mpinit(void)
{
  uchar *p, *e;
  int ismp;
  struct mp *mp;
  struct mpconf *conf;
  struct mpproc *proc;
  struct mpioapic *ioapic;

  if((conf = mpconfig(&mp)) == 0)
    panic("Expect to run on an SMP");
  ismp = 1;
  lapic = (uint*)conf->lapicaddr;
  // int i;
  // for (i=0; i<10; i++) {
  //   cprintf("%p\n", *(((uchar*)conf) + i));
  // }

  for(p=(uchar*)(conf+1), e=(uchar*)conf+conf->length; p<e; ){
    cprintf("p=%p e=%p lapic=%p length=%d conf=%p *length=%p\n",p,e,lapic, *((ushort*)&(conf->length)), conf,
            ((ushort*)&(conf->length)));
    cprintf("*p=%d, MPPROC=%d\n",*p,MPPROC);
    switch(*p){
    case MPPROC: // 0x00
      proc = (struct mpproc*)p;
      cprintf("MPPROC initialized. ncpu=%d apicid=%d\n",ncpu,proc->apicid);
      if(ncpu < NCPU) {
        cpus[ncpu].apicid = proc->apicid;  // apicid may differ from ncpu
        ncpu++;
      }
      p += sizeof(struct mpproc);
      continue;
    case MPIOAPIC: // 0x02
      cprintf("MP I/O APIC initialized\n");
      ioapic = (struct mpioapic*)p;
      ioapicid = ioapic->apicno;
      p += sizeof(struct mpioapic);
      continue;
    case MPBUS: // 0x01
    case MPIOINTR: // 0x03
    case MPLINTR: // 0x04
      p += 8;
      cprintf("Invalid Conf:%d\n", *p);
      continue;
    default:
      ismp = 0;
      cprintf("Invalid Conf:%d\n", *p);
      break;
    }
  }
  if(!ismp)
    panic("Didn't find a suitable machine");

  if(mp->imcrp){ // NOTE(nmiki): this is false
    // Bochs doesn't support IMCR, so this doesn't run on Bochs.
    // But it would on real hardware.
    outb(0x22, 0x70);   // Select IMCR
    outb(0x23, inb(0x23) | 1);  // Mask external interrupts.
  }
}
