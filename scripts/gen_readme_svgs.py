#!/usr/bin/env python3
"""Generate README animations/illustrations for go-actorlib as self-contained SVGs.

Usage: python3 scripts/gen_readme_svgs.py assets   (or: task docs-animations)
"""
import sys
from pathlib import Path

OUT = Path(sys.argv[1])

STYLE = """
<style>
  :root{--bg:#ffffff;--panel:#f6f8fa;--border:#d0d7de;--fg:#1f2328;--muted:#59636e;
        --accent:#0969da;--ok:#1a7f37;--bad:#cf222e;--warn:#9a6700;
        --c1:#0969da;--c2:#8250df;--c3:#bf3989;--c4:#1a7f37;}
  @media (prefers-color-scheme: dark){
    :root{--bg:#0d1117;--panel:#161b22;--border:#30363d;--fg:#e6edf3;--muted:#9198a1;
          --accent:#4493f8;--ok:#3fb950;--bad:#f85149;--warn:#d29922;
          --c1:#4493f8;--c2:#a371f7;--c3:#db61a2;--c4:#3fb950;}
  }
  text{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Helvetica,Arial,sans-serif;fill:var(--fg)}
  .mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
  .bg{fill:var(--bg);stroke:var(--border)}
  .panel{fill:var(--panel);stroke:var(--border);stroke-width:1.5}
  .muted{fill:var(--muted)}
  .title{font-size:16px;font-weight:600}
  .small{font-size:12px}
  .tiny{font-size:11px}
  .slot{fill:none;stroke:var(--border);stroke-width:1.5;stroke-dasharray:4 3}
  .edge{fill:none;stroke:var(--border);stroke-width:1.5}
  .chiptext{font-size:12px;font-weight:600;fill:#ffffff}
  .ok{fill:var(--ok)} .bad{fill:var(--bad)} .warn{fill:var(--warn)} .accent{fill:var(--accent)}
  .okS{stroke:var(--ok)} .badS{stroke:var(--bad)} .accentS{stroke:var(--accent)}
</style>
"""


def svg(w, h, body, label):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}" '
            f'role="img" aria-label="{label}">\n<title>{label}</title>\n{STYLE}'
            f'<rect class="bg" x="0.5" y="0.5" width="{w-1}" height="{h-1}" rx="12"/>\n{body}\n</svg>\n')


def kt(times, dur):
    """Normalize absolute times (s) to keyTimes over dur."""
    return ";".join(f"{max(0.0, min(1.0, t / dur)):.4f}" for t in times)


def anim_translate(points, dur):
    """points: list of (t, x, y) ; holds before first / after last."""
    pts = list(points)
    if pts[0][0] > 0:
        pts.insert(0, (0, pts[0][1], pts[0][2]))
    if pts[-1][0] < dur:
        pts.append((dur, pts[-1][1], pts[-1][2]))
    vals = ";".join(f"{x:.1f} {y:.1f}" for _, x, y in pts)
    return (f'<animateTransform attributeName="transform" type="translate" dur="{dur}s" '
            f'repeatCount="indefinite" values="{vals}" keyTimes="{kt([p[0] for p in pts], dur)}"/>')


def anim_attr(attr, points, dur, discrete=False):
    pts = list(points)
    if pts[0][0] > 0:
        pts.insert(0, (0, pts[0][1]))
    if pts[-1][0] < dur:
        pts.append((dur, pts[-1][1]))
    vals = ";".join(str(v) for _, v in pts)
    mode = ' calcMode="discrete"' if discrete else ""
    if discrete:
        # discrete needs keyTimes count == values count, last value at its time
        pass
    return (f'<animate attributeName="{attr}" dur="{dur}s" repeatCount="indefinite"{mode} '
            f'values="{vals}" keyTimes="{kt([p[0] for p in pts], dur)}"/>')


def chip(label, color, w=56, h=26):
    return (f'<rect x="{-w/2}" y="{-h/2}" width="{w}" height="{h}" rx="6" style="fill:var({color})"/>'
            f'<text class="chiptext mono" x="0" y="4" text-anchor="middle">{label}</text>')


def actor_box(x, y, w, h, name, lines, extra=""):
    s = f'<rect class="panel" x="{x}" y="{y}" width="{w}" height="{h}" rx="10"/>'
    s += f'<text class="title mono" x="{x+14}" y="{y+24}">{name}</text>'
    for i, ln in enumerate(lines):
        s += f'<text class="small muted" x="{x+14}" y="{y+44+i*17}">{ln}</text>'
    return s + extra


# ---------------------------------------------------------------- mailbox
def mailbox():
    W, H, D = 860, 280, 9.0
    senders = [(70, 85), (70, 145), (70, 205)]
    slots = [520, 450, 380, 310]  # head first
    sy = 145
    ax, ay, aw, ah = 610, 70, 220, 150
    body = []
    body.append('<text class="title" x="24" y="34">Mailbox: commands are processed one at a time</text>')
    body.append('<text class="small muted" x="24" y="54">Many goroutines send, only the actor goroutine touches the entity, so no mutex is needed.</text>')
    for i, (x, y) in enumerate(senders):
        body.append(f'<circle cx="{x}" cy="{y}" r="16" class="panel"/>')
        body.append(f'<text class="tiny muted" x="{x}" y="{y+4}" text-anchor="middle">g{i+1}</text>')
        body.append(f'<path class="edge" d="M{x+18} {y} C 180 {y}, 200 {sy}, 270 {sy}" stroke-dasharray="3 4"/>')
    body.append('<text class="tiny muted" x="70" y="245" text-anchor="middle">senders</text>')
    body.append('<text class="tiny muted" x="70" y="258" text-anchor="middle">ref.Send(ctx, cmd)</text>')
    # mailbox
    body.append(f'<rect class="panel" x="272" y="{sy-28}" width="286" height="56" rx="8"/>')
    for sx in slots:
        body.append(f'<rect class="slot" x="{sx-31}" y="{sy-18}" width="62" height="36" rx="6"/>')
    body.append(f'<text class="tiny muted" x="415" y="{sy+46}" text-anchor="middle">bounded mailbox (chan, default size 16)</text>')
    body.append(f'<path class="edge" d="M558 {sy} L{ax} {sy}" marker-end=""/>')
    # actor
    body.append(actor_box(ax, ay, aw, ah, "Actor[Counter]", ["one goroutine", "owns *Counter"]))
    body.append(f'<rect x="{ax+14}" y="{ay+90}" width="{aw-28}" height="40" rx="6" style="fill:var(--bg);stroke:var(--border)"/>')
    body.append(f'<text class="small mono muted" x="{ax+24}" y="{ay+115}">c.Value =</text>')
    colors = ["--c1", "--c2", "--c3", "--c4"]
    n = 4
    P = [3.0 + k * 1.2 for k in range(n)]
    # counter values
    for v in range(n + 1):
        t_on = 0 if v == 0 else P[v - 1] + 0.7
        t_off = P[v] + 0.7 if v < n else D - 0.3
        pts = [(0, 0)] if t_on > 0 else [(0, 1)]
        if t_on > 0:
            pts += [(t_on - 0.01, 0), (t_on, 1)]
        pts += [(t_off - 0.01, 1), (t_off, 0)]
        if v == 0:
            pts += [(D - 0.31, 0), (D - 0.3, 1)]
        body.append(f'<text class="title mono accent" x="{ax+112}" y="{ay+116}" opacity="0">{v}'
                    f'{anim_attr("opacity", pts, D)}</text>')
    # busy indicator
    busy = []
    for p in P:
        busy += [(p + 0.35, 0), (p + 0.4, 1), (p + 0.75, 1), (p + 0.8, 0)]
    body.append(f'<circle cx="{ax+aw-28}" cy="{ay+22}" r="6" class="ok" opacity="0">{anim_attr("opacity", busy, D)}</circle>')
    body.append(f'<text class="tiny muted" x="{ax+aw-40}" y="{ay+26}" text-anchor="end" opacity="0">Execute'
                f'{anim_attr("opacity", busy, D)}</text>')
    # chips
    for k in range(n):
        sx0, sy0 = senders[k % 3]
        a = 0.4 + k * 0.45
        arrive = a + 0.7
        pts = [(0, sx0, sy0), (a, sx0, sy0), (a + 0.35, 270, sy), (arrive, slots[k], sy)]
        pos = k
        for j in range(k):
            pos -= 1
            pts += [(P[j], slots[pos + 1], sy), (P[j] + 0.35, slots[pos], sy)]
        pts += [(P[k], slots[0], sy), (P[k] + 0.4, ax + 110, ay + 110)]
        op = [(0, 0), (a - 0.01, 0), (a, 1), (P[k] + 0.4, 1), (P[k] + 0.75, 0)]
        body.append(f'<g>{chip("+1", colors[k])}{anim_translate(pts, D)}{anim_attr("opacity", op, D)}</g>')
    return svg(W, H, "\n".join(body), "Animation: goroutines send commands into a bounded mailbox; the actor executes them one at a time")


# ---------------------------------------------------------------- backpressure
def backpressure():
    W, H, D = 860, 280, 8.0
    sy = 150
    slots = [520, 450, 380, 310]
    ax, ay, aw, ah = 610, 75, 220, 150
    body = []
    body.append('<text class="title" x="24" y="34">Backpressure: a full mailbox makes senders wait</text>')
    body.append('<text class="small muted" x="24" y="54">If the actor falls behind, Send blocks up to the receive timeout, then fails with ErrActorReceiveTimeout.</text>')
    body.append(f'<rect class="panel" x="272" y="{sy-28}" width="286" height="56" rx="8"/>')
    colors = ["--c1", "--c2", "--c3", "--c4"]
    for i, sx in enumerate(slots):
        body.append(f'<rect class="slot" x="{sx-31}" y="{sy-18}" width="62" height="36" rx="6"/>')
        body.append(f'<g transform="translate({sx} {sy})">{chip("cmd", colors[i])}</g>')
    body.append(f'<text class="tiny warn" x="415" y="{sy+46}" text-anchor="middle" font-weight="600">mailbox full (4/4)</text>')
    body.append(f'<path class="edge" d="M558 {sy} L{ax} {sy}"/>')
    body.append(actor_box(ax, ay, aw, ah, "Actor[World]", ["busy with a slow command", ""]))
    # spinner on actor
    body.append(f'<g transform="translate({ax+aw/2} {ay+110})"><circle r="14" fill="none" style="stroke:var(--border)" stroke-width="4"/>'
                f'<path d="M0 -14 A14 14 0 0 1 14 0" fill="none" class="accentS" stroke-width="4" stroke-linecap="round">'
                f'<animateTransform attributeName="transform" type="rotate" from="0" to="360" dur="1s" repeatCount="indefinite"/></path></g>')
    # sender
    gx, gy = 70, sy
    body.append(f'<circle cx="{gx}" cy="{gy}" r="16" class="panel"/><text class="tiny muted" x="{gx}" y="{gy+4}" text-anchor="middle">g1</text>')
    body.append(f'<text class="tiny muted" x="{gx}" y="{gy+36}" text-anchor="middle">ref.Send(ctx, cmd)</text>')
    wx = 215
    # new chip: travel, wait, turn red, drop
    t_arr, t_to = 1.0, 5.0
    move = [(0, gx + 30, gy), (0.3, gx + 30, gy), (t_arr, wx, gy), (t_to, wx, gy), (t_to + 0.8, wx, gy + 70)]
    op = [(0, 0), (0.29, 0), (0.3, 1), (t_to + 0.5, 1), (t_to + 0.8, 0)]
    body.append(f'<g>{chip("cmd", "--accent")}{anim_translate(move, D)}{anim_attr("opacity", op, D)}</g>')
    redop = [(0, 0), (t_to - 0.01, 0), (t_to, 1), (t_to + 0.5, 1), (t_to + 0.8, 0)]
    body.append(f'<g>{chip("cmd", "--bad")}{anim_translate(move, D)}{anim_attr("opacity", redop, D)}</g>')
    # timer ring
    r = 22
    circ = 2 * 3.14159 * r
    ring_op = [(0, 0), (t_arr - 0.01, 0), (t_arr, 1), (t_to - 0.01, 1), (t_to, 0)]
    body.append(f'<g transform="translate({wx} {gy-52})">{anim_attr("opacity", ring_op, D)}'
                f'<circle r="{r}" fill="none" style="stroke:var(--border)" stroke-width="4"/>'
                f'<circle r="{r}" fill="none" class="badS" stroke-width="4" transform="rotate(-90)" '
                f'stroke-dasharray="{circ:.1f}" stroke-dashoffset="{circ:.1f}">'
                f'{anim_attr("stroke-dashoffset", [(t_arr, round(circ,1)), (t_to, 0)], D)}</circle>'
                f'<text class="tiny muted" x="0" y="4" text-anchor="middle">wait</text></g>')
    waitlbl = [(0, 0), (t_arr - 0.01, 0), (t_arr, 1), (t_to - 0.01, 1), (t_to, 0)]
    body.append(f'<text class="tiny muted" x="{wx}" y="{gy+36}" text-anchor="middle" opacity="0">blocked (receiveTimeout)'
                f'{anim_attr("opacity", waitlbl, D)}</text>')
    errlbl = [(0, 0), (t_to - 0.01, 0), (t_to, 1), (D - 0.4, 1), (D - 0.2, 0)]
    body.append(f'<text class="small mono bad" x="{wx}" y="{gy-48}" text-anchor="middle" font-weight="600" opacity="0">ErrActorReceiveTimeout'
                f'{anim_attr("opacity", errlbl, D)}</text>')
    body.append(f'<text class="tiny muted" x="24" y="{H-18}">Tune with actor.WithInputBufferSize and actor.WithReceiveTimeout, or route failures to a dead-letter queue.</text>')
    return svg(W, H, "\n".join(body), "Animation: a full mailbox blocks the sender until the receive timeout expires")


# ---------------------------------------------------------------- supervision
def supervision():
    W, H, D = 860, 330, 8.0
    body = []
    body.append('<text class="title" x="24" y="34">Supervision: failed children are restarted (OneForOne)</text>')
    body.append('<text class="small muted" x="24" y="54">A panic is terminal for the actor; Done() closes, the supervisor notices and starts a fresh instance.</text>')
    sx, sy, sw, sh = 330, 75, 200, 60
    body.append(f'<rect class="panel" x="{sx}" y="{sy}" width="{sw}" height="{sh}" rx="10"/>')
    body.append(f'<text class="title mono" x="{sx+sw/2}" y="{sy+26}" text-anchor="middle">Supervisor</text>')
    body.append(f'<text class="tiny muted" x="{sx+sw/2}" y="{sy+45}" text-anchor="middle">max 3 restarts / 5s</text>')
    kids = [140, 430, 720]
    cy, cw, ch = 190, 190, 80
    t_panic, t_dead, t_new = 2.0, 3.2, 4.4
    for i, kx in enumerate(kids):
        body.append(f'<path class="edge" d="M{sx+sw/2} {sy+sh} C {sx+sw/2} {sy+sh+30}, {kx} {cy-30}, {kx} {cy}"/>')
        name = f"worker-{i+1}"
        if i != 1:
            body.append(f'<rect class="panel" x="{kx-cw/2}" y="{cy}" width="{cw}" height="{ch}" rx="10"/>')
            body.append(f'<text class="title mono" x="{kx}" y="{cy+28}" text-anchor="middle">{name}</text>')
            body.append(f'<circle cx="{kx-38}" cy="{cy+52}" r="5" class="ok"/>')
            body.append(f'<text class="small" x="{kx-28}" y="{cy+56}">Started</text>')
            continue
        # failing child: old instance
        old_op = [(0, 1), (t_dead, 1), (t_dead + 0.5, 0), (D - 0.5, 0), (D - 0.01, 0), (D, 1)]
        bad_border = [(0, 0), (t_panic - 0.01, 0), (t_panic, 1), (t_dead + 0.5, 1), (t_dead + 0.51, 0)]
        g = [f'<g>{anim_attr("opacity", old_op, D)}']
        g.append(f'<rect class="panel" x="{kx-cw/2}" y="{cy}" width="{cw}" height="{ch}" rx="10"/>')
        g.append(f'<rect x="{kx-cw/2}" y="{cy}" width="{cw}" height="{ch}" rx="10" fill="none" class="badS" stroke-width="3" opacity="0">'
                 f'{anim_attr("opacity", bad_border, D)}</rect>')
        g.append(f'<text class="title mono" x="{kx}" y="{cy+28}" text-anchor="middle">{name}</text>')
        ok_op = [(0, 1), (t_panic - 0.01, 1), (t_panic, 0), (D - 0.01, 0), (D, 1)]
        g.append(f'<g>{anim_attr("opacity", ok_op, D)}<circle cx="{kx-38}" cy="{cy+52}" r="5" class="ok"/>'
                 f'<text class="small" x="{kx-28}" y="{cy+56}">Started</text></g>')
        g.append(f'<g opacity="0">{anim_attr("opacity", bad_border, D)}<circle cx="{kx-42}" cy="{cy+52}" r="5" class="bad"/>'
                 f'<text class="small bad" x="{kx-32}" y="{cy+56}" font-weight="600">Panicked</text></g>')
        g.append('</g>')
        body.append("".join(g))
        # explosion text
        boom = [(0, 0), (t_panic - 0.01, 0), (t_panic, 1), (t_dead, 1), (t_dead + 0.3, 0)]
        body.append(f'<text class="small mono bad" x="{kx}" y="{cy+ch+22}" text-anchor="middle" opacity="0">panic: nil map write'
                    f'{anim_attr("opacity", boom, D)}</text>')
        # restart arrow pulse along edge
        pulse = [(0, 0), (t_dead + 0.2, 0), (t_dead + 0.3, 1), (t_new, 1), (t_new + 0.1, 0)]
        body.append(f'<circle r="6" class="accent" opacity="0">{anim_attr("opacity", pulse, D)}'
                    f'<animateMotion dur="{D}s" repeatCount="indefinite" path="M{sx+sw/2} {sy+sh} C {sx+sw/2} {sy+sh+30}, {kx} {cy-30}, {kx} {cy}" '
                    f'keyPoints="0;0;1;1" keyTimes="0;{(t_dead+0.3)/D:.4f};{t_new/D:.4f};1" calcMode="linear"/></circle>')
        # new instance
        new_op = [(0, 0), (t_new - 0.01, 0), (t_new + 0.4, 1), (D - 0.5, 1), (D, 0)]
        body.append(f'<g opacity="0">{anim_attr("opacity", new_op, D)}'
                    f'<rect class="panel" x="{kx-cw/2}" y="{cy}" width="{cw}" height="{ch}" rx="10"/>'
                    f'<rect x="{kx-cw/2}" y="{cy}" width="{cw}" height="{ch}" rx="10" fill="none" class="okS" stroke-width="3"/>'
                    f'<text class="title mono" x="{kx}" y="{cy+28}" text-anchor="middle">{name}</text>'
                    f'<circle cx="{kx-52}" cy="{cy+52}" r="5" class="ok"/>'
                    f'<text class="small" x="{kx-42}" y="{cy+56}">Started (restart 1/3)</text></g>')
        lbl = [(0, 0), (t_dead + 0.2, 0), (t_dead + 0.3, 1), (t_new + 0.3, 1), (t_new + 0.5, 0)]
        body.append(f'<text class="tiny muted" x="{sx+sw+16}" y="{sy+sh+22}" opacity="0">death watch: Done() closed → restart'
                    f'{anim_attr("opacity", lbl, D)}</text>')
    body.append(f'<text class="tiny muted" x="24" y="{H-14}">Siblings keep running. A clean Stop (state Done) is not restarted; AllForOne restarts every child.</text>')
    return svg(W, H, "\n".join(body), "Animation: a supervisor restarts a child actor after it panics")


# ---------------------------------------------------------------- hero
def hero():
    W, H, D = 860, 330, 8.0
    body = []
    body.append('<text class="title" x="24" y="34">Every entity is an actor</text>')
    body.append('<text class="small muted" x="24" y="54">Its own goroutine, its own mailbox, private state. Entities interact only by sending messages.</text>')
    boxes = {
        "World": (330, 78, 200, 92, ["players: 1, monsters: 1"]),
        "Player": (60, 210, 200, 92, None),
        "Monster": (600, 210, 200, 92, None),
    }
    for name, (x, y, w, h, lines) in boxes.items():
        body.append(f'<rect class="panel" x="{x}" y="{y}" width="{w}" height="{h}" rx="10"/>')
        body.append(f'<text class="title mono" x="{x+14}" y="{y+24}">Actor[{name}]</text>')
        if lines:
            body.append(f'<text class="small muted" x="{x+14}" y="{y+46}">{lines[0]}</text>')
        # mailbox strip
        body.append(f'<text class="tiny muted" x="{x+14}" y="{y+76}">mailbox</text>')
        for i in range(4):
            body.append(f'<rect class="slot" x="{x+70+i*26}" y="{y+64}" width="20" height="16" rx="3"/>')
    # hp counters (state changes)
    def hp(x, y, before, after, t):
        a = [(0, 1), (t - 0.01, 1), (t, 0), (D - 0.01, 0), (D, 1)]
        b = [(0, 0), (t - 0.01, 0), (t, 1), (D - 0.3, 1), (D - 0.29, 0)]
        return (f'<text class="small muted" x="{x}" y="{y}">hp: <tspan class="mono">{before}</tspan>{anim_attr("opacity", a, D)}</text>'
                f'<text class="small" x="{x}" y="{y}" opacity="0">hp: <tspan class="mono bad">{after}</tspan>{anim_attr("opacity", b, D)}</text>')
    msgs = [
        # label, color, path, start, end, target box
        ("Move", "--c1", "M200 210 C 220 150, 270 125, 330 125", 0.4, 1.6, "World"),
        ("Moved", "--c2", "M530 125 C 590 125, 650 150, 680 210", 2.0, 3.2, "Monster"),
        ("Attack", "--c3", "M600 270 C 480 300, 380 300, 260 270", 3.6, 5.0, "Player"),
        ("Hit", "--c4", "M260 240 C 380 215, 480 215, 600 240", 5.4, 6.8, "Monster"),
    ]
    body.append(hp(74, 256, 100, 88, 5.2))
    body.append(hp(614, 256, 50, 20, 7.0))
    for label, color, path, t0, t1, tgt in msgs:
        body.append(f'<path class="edge" d="{path}" stroke-dasharray="3 4"/>')
        op = [(0, 0), (t0 - 0.01, 0), (t0, 1), (t1, 1), (t1 + 0.2, 0)]
        body.append(f'<g opacity="0">{chip(label, color, w=64)}{anim_attr("opacity", op, D)}'
                    f'<animateMotion dur="{D}s" repeatCount="indefinite" path="{path}" calcMode="linear" '
                    f'keyPoints="0;0;1;1" keyTimes="0;{t0/D:.4f};{t1/D:.4f};1"/></g>')
        x, y, w, h, _ = boxes[tgt]
        slot = [(0, 0), (t1 - 0.01, 0), (t1, 1), (t1 + 0.5, 1), (t1 + 0.6, 0)]
        body.append(f'<rect x="{x+70}" y="{y+64}" width="20" height="16" rx="3" style="fill:var({color})" opacity="0">'
                    f'{anim_attr("opacity", slot, D)}</rect>')
        flash = [(0, 0), (t1 - 0.01, 0), (t1, 1), (t1 + 0.6, 0)]
        body.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="10" fill="none" class="accentS" stroke-width="2.5" opacity="0">'
                    f'{anim_attr("opacity", flash, D)}</rect>')
    return svg(W, H, "\n".join(body), "Animation: Player, World and Monster actors exchanging messages through their mailboxes")


# ---------------------------------------------------------------- tradeoffs (static)
def tradeoffs():
    W, H = 860, 300
    pros = ["Long-lived entities: players, sessions, devices", "State that many goroutines read and mutate",
            "Event-driven messaging between entities", "Ordered startup and coordinated shutdown",
            "Isolate failures, restart only what broke", "Bursty load that needs backpressure"]
    cons = ["Request-scoped work → errgroup / WaitGroup", "A single counter or flag → sync/atomic",
            "Read-heavy, write-rare state → sync.RWMutex", "CPU-bound pipelines → worker pools",
            "Actors across processes or machines", "Durable state, unless you add persistence"]
    body = []
    colw = 400
    for ci, (title, items, cls, mark) in enumerate([("Good fit", pros, "ok", "✓"), ("Not a fit", cons, "bad", "✕")]):
        x = 20 + ci * (colw + 20)
        body.append(f'<rect class="panel" x="{x}" y="20" width="{colw}" height="{H-40}" rx="10"/>')
        body.append(f'<text class="title {cls}" x="{x+20}" y="52">{title}</text>')
        for i, it in enumerate(items):
            y = 88 + i * 32
            body.append(f'<text class="{cls}" x="{x+22}" y="{y}" font-weight="700" font-size="14">{mark}</text>')
            body.append(f'<text class="small" x="{x+44}" y="{y}" font-size="13">{it}</text>')
    return svg(W, H, "\n".join(body), "When go-actorlib is a good fit and when it is not")


(OUT / "animations").mkdir(parents=True, exist_ok=True)
(OUT / "illustrations").mkdir(parents=True, exist_ok=True)
(OUT / "animations/actors-messaging.svg").write_text(hero())
(OUT / "animations/mailbox.svg").write_text(mailbox())
(OUT / "animations/backpressure.svg").write_text(backpressure())
(OUT / "animations/supervision.svg").write_text(supervision())
(OUT / "illustrations/fit.svg").write_text(tradeoffs())
print("ok")
