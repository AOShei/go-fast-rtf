"""Run a command and report its wall time, CPU time and peak memory.

    python -I measure.py OUT.json -- CMD ARGS...

Peak memory is the largest sum of the resident sizes of the process and
all its descendants, sampled every 20 ms (psutil). CPU time is user plus
system of the process and its waited-for children (os.wait4).

The machine is shared, so a run waits until it is quiet (all cores
together less than 5% busy over 3 s, for up to an hour) and records the
CPU time other processes used while it ran beyond that background
(others_cpu_s: the machine's busy time from /proc/stat less the run's own, this script's sampling
and the background rate of the quiet window). A run during which others
used more than 5% of the run's own CPU time, or more than 2 s, is
repeated, up to 5 times, waiting for quiet again each time.
"""
import json, os, subprocess, sys, threading, time
import psutil

out = sys.argv[1]
cmd = sys.argv[sys.argv.index("--") + 1:]


def busy():
    # Busy CPU seconds of the whole machine so far.
    with open("/proc/stat") as f:
        v = [int(x) for x in f.readline().split()[1:]]
    idle = v[3] + v[4]
    return (sum(v) - idle) / os.sysconf("SC_CLK_TCK")


def wait_quiet():
    # Waits for quiet; returns the background rate then, in busy cores.
    deadline = time.time() + 3600
    n = os.cpu_count()
    rate = 0.0
    while time.time() < deadline:
        b0 = busy()
        time.sleep(3)
        rate = (busy() - b0) / 3
        if rate / n < 0.05:
            return rate
    print("the machine did not become quiet within an hour", file=sys.stderr)
    return rate


for attempt in range(5):
    background = wait_quiet()
    m0 = busy()
    self0 = time.process_time()  # this script's own CPU, its sampler's
    t0 = time.perf_counter()
    p = subprocess.Popen(cmd)
    peak = 0
    done = False


    def sample():
        global peak
        pp = psutil.Process(p.pid)
        while not done:
            try:
                rss = pp.memory_info().rss + sum(c.memory_info().rss for c in pp.children(recursive=True))
                peak = max(peak, rss)
            except psutil.Error:
                pass
            time.sleep(0.02)


    th = threading.Thread(target=sample, daemon=True)
    th.start()
    _, status, ru = os.wait4(p.pid, 0)
    wall = time.perf_counter() - t0
    done = True
    th.join()
    cpu = ru.ru_utime + ru.ru_stime
    # What others used beyond the background of the quiet window.
    others = max(busy() - m0 - cpu - (time.process_time() - self0) - background * wall, 0.0)
    if others <= max(0.05 * cpu, 2.0) or attempt == 4:
        break
    print(f"others used {others:.1f} s of CPU during the run; repeating", file=sys.stderr)
json.dump({"cmd": cmd, "status": os.waitstatus_to_exitcode(status), "wall_s": wall,
           "cpu_s": cpu, "peak_rss_mb": peak / 2**20, "others_cpu_s": others,
           "attempts": attempt + 1}, open(out, "w"))
print(f"{wall:.2f} s wall, {cpu:.2f} s CPU, {peak / 2**20:.0f} MB peak, others {others:.1f} s CPU")
