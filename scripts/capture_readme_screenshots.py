"""Capture README screenshots at the sizes already used in screenshots/.

Requires a running dashboard on the default ports:

    go build -o srdashboard.exe .
    ./srdashboard.exe
    python scripts/capture_readme_screenshots.py

Hall shots: 1920x1080. Shooter: 1280x800. QR dialog is cropped to the modal.

Release captures must not burst UDP. OpticScore datagrams vanish if the next
packet arrives before the listener has applied the last one; games then skip
shots, finish too soon, or show unequal shot-count warnings. Default gap is
250ms and can be raised with SRDASH_SHOT_GAP.

Autorennen is round-based: send one scoring shot per stand, then wait until
currentRound advances before the next round. Do not stack the next shot on a
lane that has not finished this round.

Classic Range README shots mix LG / LP / KK (and Auflage) across the six
stands so the hall shows three target faces. Stand 1 stays LG 40 for the
shooter screenshot. Other games keep DiscType LG.
"""
from __future__ import annotations

import argparse
import os
import shutil
import sys
import time
from pathlib import Path

from playwright.sync_api import sync_playwright

sys.path.insert(0, str(Path(__file__).resolve().parent))
import ui_regression as u  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
OUT = Path(os.environ.get("SRDASH_SCREENSHOT_DIR", ROOT / "screenshots"))
HALL = {"width": 1920, "height": 1080}
SHOOTER = {"width": 1280, "height": 800}

NAMES = [
    ("Alex", "Alpha"),
    ("Blake", "Bravo"),
    ("Casey", "Charlie"),
    ("Dana", "Delta"),
    ("Eden", "Echo"),
    ("Finn", "Foxtrot"),
]

u.BASE = os.environ.get("SRDASH_URL", "http://127.0.0.1:8080")
u.UDP = ("127.0.0.1", int(os.environ.get("SRDASH_UDP", "30169")))
u.SHOT_GAP = float(os.environ.get("SRDASH_SHOT_GAP", "0.25"))


# Per-stand OpticScore programs for Classic Range hall captures (mixed faces).
# Index is range number minus one. LP/KK must use band 80 or ValidateShot drops them.
CLASSIC_RANGE_PROGRAMS = [
    {"disc": "LG", "menu": "LG 40 Schuss", "band": 25.0, "shots": 40, "mean": 10.34},
    {"disc": "LG", "menu": "LG 30 Schuss Auflage", "band": 25.0, "shots": 30, "mean": 10.55},
    {"disc": "LP", "menu": "LP 40 Schuss", "band": 80.0, "shots": 40, "mean": 9.12},
    {"disc": "LP", "menu": "LP 40 Schuss Auflage", "band": 80.0, "shots": 40, "mean": 9.72},
    {"disc": "KK", "menu": "KK 40 Schuss", "band": 80.0, "shots": 40, "mean": 8.45},
    {"disc": "KK", "menu": "KK 40 Schuss Auflage", "band": 80.0, "shots": 40, "mean": 9.45},
]


def send_shot(range_num, dec, warmup=False, n=0, menu="LG 40 Schuss", disc="LG", band=25.0):
    first, last = NAMES[range_num - 1]
    x, y, distance = u.shot_xy(dec, n, range_num, band=band)
    before = u.live_shot_number(range_num)
    obj = {
        "X": x, "Y": y, "Distance": distance,
        "FullValue": min(10, max(0, int(dec))), "DecValue": dec,
        "Range": range_num, "IsWarmup": warmup, "DiscType": disc,
        "Shooter": {"Firstname": first, "Lastname": last},
        "MenuItem": {"MenuItemName": menu, "MenuPointName": menu},
    }
    u.send_raw_shot(obj)
    deadline = time.time() + 3.0
    applied = False
    after = before
    while time.time() < deadline:
        after = u.live_shot_number(range_num)
        # Warmup→competition resets ShotNumber, so a drop also means applied.
        if after > before or (before > 0 and after >= 0 and after < before):
            applied = True
            break
        time.sleep(0.05)
    if not applied:
        print("shot not applied range=%d dec=%s n=%s before=%s after=%s" % (
            range_num, dec, n, before, after))
    time.sleep(u.SHOT_GAP)


def live_field(warmup=False, shots=10):
    return {
        str(r): {
            "shooterName": "%s %s" % NAMES[r - 1],
            "discipline": "LG %d Schuss" % shots,
            "isWarmup": warmup,
            "totalShotsToFire": shots,
        }
        for r in range(1, u.RANGES + 1)
    }


u.send_shot = send_shot
u.live_field = live_field


def put_plugin_config(plugin_id, overrides):
    u.api("PUT", "/api/plugins/%s/config" % plugin_id, {"overrides": overrides})


def wait_paint(page):
    page.wait_for_function(
        """() => {
          const shared = document.querySelector('#shared-master-host:not([hidden])');
          const shooter = document.querySelector('#shooter-plugin-view');
          const panel = document.querySelector('.range-panel:not([hidden])');
          const host = shared || shooter || (panel && (panel.querySelector('.range-plugin-view') || panel));
          if (!host) return false;
          const text = (host.innerText || '').replace(/\\s+/g, ' ').trim();
          const hasSvg = !!host.querySelector('svg');
          return host.children.length > 0 && (text.length > 10 || hasSvg);
        }""",
        timeout=20000,
    )


def goto(page, path):
    page.goto(u.BASE + path, wait_until="domcontentloaded", timeout=30000)
    wait_paint(page)
    page.wait_for_timeout(250)


def active_ids():
    data = u.api("GET", "/api/plugins/active")
    if isinstance(data, list):
        return [x.get("id") for x in data]
    if isinstance(data, dict) and data.get("id"):
        return [data.get("id")]
    return []


def assert_active(plugin_id):
    ids = active_ids()
    print("active", ids)
    if plugin_id not in ids:
        raise RuntimeError("expected plugin %s, got %s" % (plugin_id, ids))


def shot(page, name, jpeg=False):
    staging = OUT / ".staging"
    staging.mkdir(parents=True, exist_ok=True)
    tmp = staging / name
    dest = OUT / name
    kwargs = {"path": str(tmp), "full_page": False}
    if jpeg:
        kwargs["type"] = "jpeg"
        kwargs["quality"] = 82
    page.screenshot(**kwargs)
    try:
        shutil.copyfile(tmp, dest)
        print("wrote", dest, dest.stat().st_size)
    except OSError as exc:
        alt = dest.with_name(dest.stem + "-new" + dest.suffix)
        shutil.copyfile(tmp, alt)
        print("dest locked (%s); wrote" % exc, alt, alt.stat().st_size)


def start_game(plugin_id, shots=10, warmup=False):
    u.activate(plugin_id)
    u.control("reset")
    u.control("sync_live", {"live": live_field(warmup=warmup, shots=shots)})
    try:
        u.control("start", {"live": live_field(warmup=False, shots=shots)})
    except Exception as exc:
        print(plugin_id, "start:", exc)
    time.sleep(0.35)


def fill_classic():
    """Probe series + mixed LG/LP/KK Wertung, round-robin."""
    u.activate("classic-range")
    u.reset_live()
    warmup_n = 10
    programs = CLASSIC_RANGE_PROGRAMS
    max_comp = max(p["shots"] for p in programs)

    def dec_for(range_num, i, warmup):
        mean = programs[range_num - 1]["mean"] - (0.35 if warmup else 0.0)
        wobble = ((i * 17 + range_num * 13) % 21) / 10.0 - 1.0
        return round(max(6.5, min(10.9, mean + wobble)), 1)

    def fire(range_num, dec, warmup, n):
        prog = programs[range_num - 1]
        send_shot(
            range_num, dec, warmup=warmup, n=n,
            menu=prog["menu"], disc=prog["disc"], band=prog["band"],
        )

    print("classic mixed disciplines:")
    for r, prog in enumerate(programs, 1):
        print("  stand %d: %s [%s] %d wertung" % (r, prog["menu"], prog["disc"], prog["shots"]))
    for i in range(warmup_n):
        for r in range(1, u.RANGES + 1):
            fire(r, dec_for(r, i, True), True, i + r * 3)
        if (i + 1) % 5 == 0:
            print("  warmup %d/%d" % (i + 1, warmup_n))
    for i in range(max_comp):
        for r in range(1, u.RANGES + 1):
            if i >= programs[r - 1]["shots"]:
                continue
            fire(r, dec_for(r, i, False), False, 100 + i + r * 3)
        if (i + 1) % 10 == 0:
            print("  competition %d/%d" % (i + 1, max_comp))


def capture_classic(hall, shooter, with_qr=True, fill=True):
    if fill:
        fill_classic()
        assert_active("classic-range")
    goto(hall, "/")
    hall.wait_for_selector(".range-qr-btn")
    shot(hall, "classic-range-master.png")
    goto(shooter, "/1/")
    shot(shooter, "classic-range-shooter.png")

    if with_qr:
        hall.locator('.range-panel[data-range="1"] .range-qr-btn').click()
        hall.wait_for_selector("#qr-result-img")
        hall.wait_for_function(
            "() => { const i = document.getElementById('qr-result-img'); return i && i.complete && i.naturalWidth > 0; }",
            timeout=15000,
        )
        hall.wait_for_timeout(200)
        shot(hall, "classic-range-qr.png")
        hall.locator(".qr-result-dialog").screenshot(path=str(OUT / ".staging" / "classic-range-qr-dialog.png"))
        dialog_src = OUT / ".staging" / "classic-range-qr-dialog.png"
        dialog_dest = OUT / "classic-range-qr-dialog.png"
        try:
            shutil.copyfile(dialog_src, dialog_dest)
            print("wrote", dialog_dest, dialog_dest.stat().st_size)
        except OSError as exc:
            alt = OUT / "classic-range-qr-dialog-new.png"
            shutil.copyfile(dialog_src, alt)
            print("dest locked (%s); wrote" % exc, alt)
        hall.locator(".qr-result-close").click()
        hall.wait_for_timeout(150)

    u.activate("classic-range-condensed")
    assert_active("classic-range-condensed")
    goto(hall, "/")
    hall.wait_for_selector(".crc-view .range-target svg", timeout=20000)
    hall.wait_for_timeout(400)
    shot(hall, "classic-range-condensed.png")


def capture_condensed(hall):
    u.activate("classic-range-condensed")
    assert_active("classic-range-condensed")
    goto(hall, "/")
    hall.wait_for_selector(".crc-view .range-target svg", timeout=20000)
    hall.wait_for_timeout(500)
    shot(hall, "classic-range-condensed.png")


def capture_menu(hall):
    u.activate("classic-range")
    goto(hall, "/")
    hall.locator("#menu-toggle").click()
    hall.wait_for_timeout(350)
    shot(hall, "classic-range-menu.png")
    hall.locator("#menu-toggle").click()


def race_state():
    _, vm = u.phase_of(u.session())
    return vm.get("race") or {}


def wait_autorennen_round(min_round, timeout=12):
    """Block until every racing car has finished the previous round."""
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        race = race_state()
        last = race
        cur = int(race.get("currentRound") or 0)
        if cur >= min_round:
            return race
        time.sleep(0.12)
    print("timeout waiting for Autorennen round", min_round,
          "got", (last or {}).get("currentRound"),
          "blocked", (last or {}).get("startBlockedReason"))
    return last or {}


def send_autorennen_round(values, n, warmup=False):
    for r in range(1, u.RANGES + 1):
        send_shot(r, values[r - 1], warmup=warmup, n=n, menu="LG 10 Schuss")


def capture_autorennen(hall):
    put_plugin_config("autorennen", {"circuitId": "hafenpark", "fieldEventsEnabled": False})
    u.reset_live()
    u.activate("autorennen")
    u.control("reset")
    u.control("sync_live", {"live": live_field(warmup=True, shots=10)})
    send_autorennen_round([8.5] * u.RANGES, n=1, warmup=True)
    u.control("start", {"live": live_field(warmup=False, shots=10)})
    time.sleep(0.6)
    pace = [9.4, 9.9, 8.7, 9.2, 6.6, 5.7]
    # Round 1 is the grid shot; CurrentRound becomes 2 when every stand has fired.
    send_autorennen_round(pace, n=10)
    wait_autorennen_round(2)
    time.sleep(0.35)
    for rnd in range(2, 7):
        send_autorennen_round(pace, n=10 + rnd)
        wait_autorennen_round(rnd + 1)
        time.sleep(0.35)
    assert_active("autorennen")
    goto(hall, "/")
    hall.wait_for_selector(".ar-track-host svg", timeout=15000)
    hall.wait_for_function(
        """() => {
          const img = document.querySelector('.ar-track-host svg image');
          if (!img) return false;
          const href = (img.href && img.href.baseVal) || img.getAttribute('href') || '';
          if (href.indexOf('hafenpark-bg') < 0) return false;
          return performance.getEntriesByType('resource').some((e) => (
            e.name.indexOf('hafenpark-bg') >= 0 && (e.encodedBodySize || e.decodedBodySize || 0) > 1000
          ));
        }""",
        timeout=15000,
    )
    hall.wait_for_timeout(500)
    body = hall.inner_text("body")
    if "Unterschiedliche" in body:
        print("autorennen still showing shot-count warning; capturing anyway")
    shot(hall, "autorennen.jpg", jpeg=True)
    put_plugin_config("autorennen", {})


def capture_fox(hall):
    u.activate("fox-on-the-run")
    u.control("reset")
    u.control("sync_live", {"live": live_field()})
    for r in range(1, u.RANGES + 1):
        for i in range(5):
            send_shot(r, 8.2 + (r % 3) * 0.3, n=i + r * 5)
    try:
        u.control("start", {"live": live_field()})
    except Exception as exc:
        print("fox start:", exc)
    time.sleep(0.4)
    for i in range(18):
        ph, vm = u.phase_of(u.session())
        hunt = vm.get("hunt") or {}
        turn = hunt.get("turnRange")
        fox = hunt.get("currentFox")
        phase = (hunt.get("phase") or ph or "").lower()
        if "finish" in phase:
            break
        if not turn:
            try:
                u.control("start", {"live": live_field()})
            except Exception:
                pass
            time.sleep(0.05)
            continue
        dec = 9.2 if turn == fox else 10.5
        send_shot(int(turn), dec, n=100 + i)
    goto(hall, "/")
    hall.wait_for_selector(".fox-revier-wrap svg, .fox-scheibe-wrap svg", timeout=15000)
    hall.wait_for_timeout(300)
    shot(hall, "fox-on-the-run.jpg", jpeg=True)


def capture_tannebaum(hall, plugin_id, filename):
    seq = [5, 6, 7, 8.0, 8.5, 9.0, 9.5, 10.0]
    start_game(plugin_id, shots=40)
    for r in range(1, u.RANGES + 1):
        for i, v in enumerate(seq):
            send_shot(r, v, n=i + r * 10)
    goto(hall, "/")
    hall.wait_for_timeout(300)
    shot(hall, filename)


def capture_ludo(hall):
    start_game("ludo", shots=40)
    for r in range(1, u.RANGES + 1):
        send_shot(r, 10.0, n=1)
    for n in range(6):
        for r in range(1, u.RANGES + 1):
            send_shot(r, 10.5 if (n + r) % 3 else 9.5, n=10 + n)
        time.sleep(0.3)
    assert_active("ludo")
    goto(hall, "/")
    hall.wait_for_selector(".ld-board-svg", timeout=15000)
    hall.wait_for_timeout(300)
    shot(hall, "ludo.png")


def capture_barrikade(hall):
    start_game("barrikade", shots=40)
    for n in range(10):
        for r in range(1, u.RANGES + 1):
            send_shot(r, 10.0 if n % 4 == 3 else 10.5, n=n)
    goto(hall, "/")
    hall.wait_for_timeout(300)
    shot(hall, "barrikade.png")


def capture_bingo(hall):
    start_game("zehner-bingo", shots=40)
    _, vm = u.phase_of(u.session())
    values = ((vm or {}).get("game") or {}).get("cardValues") or []
    for i, v in enumerate(values[:8]):
        send_shot(1, float(v), n=i)
    for r in range(2, u.RANGES + 1):
        for i, v in enumerate(values[:3]):
            send_shot(r, float(v), n=20 + i)
    goto(hall, "/")
    hall.wait_for_timeout(300)
    shot(hall, "zehner-bingo.png")
    goto(hall, "/compact")
    hall.wait_for_timeout(300)
    shot(hall, "zehner-bingo-compact.png")


def capture_recovery(page):
    page.goto(u.BASE + "/config", wait_until="domcontentloaded", timeout=30000)
    page.wait_for_selector("#recovery-log", timeout=15000)
    loc = page.locator("section.config-section").filter(has_text="Wiederherstellung")
    loc.scroll_into_view_if_needed()
    page.wait_for_timeout(200)
    loc.screenshot(path=str(OUT / "config-recovery.png"))
    print("wrote", OUT / "config-recovery.png")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--only", default="", help="comma list: classic,autorennen,ludo,...")
    args = parser.parse_args()
    only = {x.strip() for x in args.only.split(",") if x.strip()}

    def want(*names):
        return not only or any(n in only for n in names)

    OUT.mkdir(parents=True, exist_ok=True)
    try:
        u.api("GET", "/api/plugins")
    except Exception as exc:
        print("dashboard not running at", u.BASE, "—", exc, file=sys.stderr)
        return 2

    with sync_playwright() as p:
        try:
            browser = p.chromium.launch(channel="msedge", headless=True, args=["--disable-gpu"])
        except Exception:
            browser = p.chromium.launch(headless=True)
        hall = browser.new_page(viewport=HALL)
        shooter = browser.new_page(viewport=SHOOTER)
        try:
            if want("classic"):
                capture_classic(hall, shooter, with_qr=True, fill=True)
            elif want("condensed"):
                capture_condensed(hall)
            if want("classic-views"):
                capture_classic(hall, shooter, with_qr=True, fill=False)
            if want("menu"):
                capture_menu(hall)
            if want("autorennen"):
                capture_autorennen(hall)
            if want("fox"):
                capture_fox(hall)
            if want("tannebaum-einzel"):
                capture_tannebaum(hall, "tannebaum-einzel", "tannebaum-einzel.png")
            if want("tannebaum-team"):
                capture_tannebaum(hall, "tannebaum-team", "tannebaum-team.png")
            if want("ludo"):
                capture_ludo(hall)
            if want("barrikade"):
                capture_barrikade(hall)
            if want("bingo"):
                capture_bingo(hall)
            if want("recovery"):
                capture_recovery(hall)
            if not only:
                u.activate("classic-range")
                u.reset_live()
        finally:
            browser.close()
    print("done")
    return 0


if __name__ == "__main__":
    sys.exit(main())
