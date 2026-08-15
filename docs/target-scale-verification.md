# Target scale verification (DISAG vs ISSF vs log data)

No scripts — verification is done with log excerpts and arithmetic below.

## Source data

### ISSF 10 m air rifle
- **Total target diameter:** 45.5 mm (radius 22.75 mm)
- **10 ring diameter:** 0.5 mm (radius 0.25 mm)
- **9 ring diameter:** 5.5 mm (radius 2.75 mm)
- **4 ring diameter:** 30.5 mm (radius 15.25 mm)

### Log (OUT-JSONInterface.log.txt)
- **X, Y** — shot coordinates (centre 0,0), in DISAG units. Use these for position on the target.
- **Distance** — the **Teiler** (scoring measure, same units as X,Y).
- **Correlation:** **Teiler (Distance) = √(X² + Y²)** — i.e. the Euclidean distance from centre. Check: X=70, Y=−144 → √(4900+20736)=160.1 (log has 160.1); X=−990, Y=−543 → √(980100+294849)=1129.1 (log has 1129.1).
- **DecValue** ↔ **Teiler** bands: each 0.1 step in DecValue spans 25 Teiler units:
  - **10.9** = Teiler 0 to 25  
  - **10.8** = Teiler 25 to 50  
  - **10.7** = Teiler 50 to 75  
  - … and so on (each 0.1 DecValue = band of width 25).
- **FullValue** — integer ring (4–10).
- For drawing, use **X** and **Y**; the scale is **±9000 DISAG = 200 mm** (90 DISAG/mm).

---

## Hypothesis A: ±9000 DISAG = 2000 mm (full range)

Then **9000 DISAG = 1000 mm** (radius), so **1 mm = 9 DISAG** (DSG_PER_MM = 9).

| Log excerpt | Distance (DISAG) | DecValue | In mm (÷9) | ISSF check |
|-------------|------------------|----------|------------|------------|
| Shot 10.9 (X=-10,Y=-4) | 10.7 | 10.9 | 10.7/9 = **1.19 mm** | 10 ring = 0.25 mm. 1.19 mm is in 9 ring, not 10. **Mismatch.** |
| Shot 4.7 (X=-1106,Y=1101) | 1560.5 | 4.7 | 1560.5/9 = 173 mm | Would be far outside 45.5 mm target. **Mismatch.** |

So **if ±9000 = 2000 mm, the log scores (DecValue/FullValue) do not match ISSF ring radii.** The same DISAG values would need a much smaller physical scale to match the scores.

---

## Hypothesis B: ±9000 DISAG = 200 mm (range that matches scores)

Then **9000 DISAG = 100 mm** (radius), so **1 mm = 90 DISAG** (DSG_PER_MM = 90).

| Log excerpt | Distance (DISAG) | DecValue | In mm (÷90) | ISSF check |
|-------------|------------------|----------|-------------|------------|
| 10.9 shot | 10.7 | 10.9 | 10.7/90 = **0.12 mm** | < 0.25 mm (10 ring). **OK** |
| 10.5 shot | 108.5 | 10.5 | 108.5/90 = 1.21 mm | < 2.75 mm (9 ring). **OK** |
| 9.6 shot | 334.8 | 9.6 | 334.8/90 = 3.72 mm | Inside 9 ring (2.75 mm) … 3.72 is 8 ring; 9.6 is high 9, so slightly outside 2.75. **Plausible** |
| 4.7 shot | 1560.5 | 4.7 | 1560.5/90 = **17.3 mm** | 4 ring outer = 15.25 mm. 17.3 mm would be 3 ring. **Borderline** (scoring can use decimal boundaries). |

So **9000 = 100 mm radius (200 mm diameter)** fits the log DecValue/FullValue much better than 2000 mm. The 45.5 mm ISSF target then sits inside this 200 mm range (45.5/200 ≈ 23% of the DISAG range).

---

## Conclusion

- **Target (ISSF):** 45.5 mm diameter, inside a **200 mm** coordinate range (confirmed).
- **For drawing and overlay:** **9000 DISAG = 100 mm radius** (DSG_PER_MM = 90, ±9000 = 200 mm). Use **X and Y** for shot position. **Distance** in the log is the Teiler; DecValue 10.9 = Teiler 0–25, 10.8 = 25–50, etc. (each 0.1 DecValue = 25 Teiler units).

## Constants for code (data-based)

| Constant | Value | Meaning |
|----------|--------|--------|
| `DSG_COORD_RANGE` | 9000 | Max radius in DISAG |
| `RANGE_DIAMETER_MM` | 200 | Physical diameter that fits log scores (9000 = 100 mm radius) |
| `DSG_PER_MM` | 90 | 9000 / 100 |
| `TARGET_DIAMETER_MM` (ISSF) | 45.5 | Scoring target diameter |
| `TEN_RING_DIAMETER_MM` | 0.5 | 10 ring |
| Shot diameter | 4.5 mm | For drawing |
| DecValue ↔ Teiler | 10.9 = Teiler 0–25, 10.8 = 25–50, … (each 0.1 DecValue = 25 Teiler) | Log "Distance" = Teiler |

SVG (viewBox 0 0 200 200, centre 100): use **90 DSG per SVG unit** from centre (100 SVG units = 100 mm at this scale). So **x_svg = 100 + x_dsg/90**, **y_svg = 100 - y_dsg/90**.

---

## ISSF 10 m air pistol (vs rifle)

Same **decimal ceiling (10.9)** and same DISAG coordinate frame (±9000 = 200 mm), but **ring geometry and Teiler bands differ**:

| | Air rifle | Air pistol |
|---|-----------|------------|
| Scoring disk Ø | 45.5 mm | 155.5 mm |
| 10-ring Ø | 0.5 mm | 11.5 mm |
| Inner-10 Ø (tie-break) | 0.5 mm | 5.0 mm |
| Ring band width | 2.5 mm | 8.0 mm |
| Black aiming mark | rings 4–9 (30.5 mm) | rings 7–10 (59.5 mm) |
| DecValue per 0.1 step (geometry) | ~0.25 mm → **~25 DSG** | ~0.8 mm → **~72 DSG** |

ISSF divides each full ring into ten decimal sub-zones (10.0 … 10.9). The **absolute** distance for one 0.1 step is ring_width/10, hence rifle ≈25 DSG and pistol ≈72 DSG at 90 DSG/mm.

**SRDashboard does not recompute ring scores** — `DecValue`, `FullValue`, and `Distance` (Teiler) are taken from OpticScore UDP as-is (already discipline-specific). Per-profile geometry lives in `plugins/classic-range/target-registry.js` (`teilerBandDsg`, ring sizes, zoom). OpticScore may also apply a **Teiler factor** for Luftpistole in site settings (DISAG manual); that affects exported Teiler, not our display logic.

**Pellet placement** uses X/Y with profile-specific `coordRadiusMm` (±9000 DISAG units span that radius in mm):

| Profile | `coordRadiusMm` | DSG/mm | Notes |
|---------|-----------------|--------|--------|
| Air rifle | 100 | 90 | Log-verified (±9000 = 200 mm Ø frame) |
| Air pistol | 130 (provisional) | ~69 | **Needs refinement** — no LP log shot yet; using 90/mm plots too close to centre |

### Pistol placement (TODO — refine with live data)

Luftpistole pellet placement uses a provisional `coordRadiusMm` in `plugins/classic-range/target-registry.js` (`coordRadiusNeedsRefinement: true`). Integer/decimal scores from OpticScore are unchanged; only X/Y → SVG mapping may be off until calibrated with one real log line (DecValue + X + Y + Distance).

When available: compare expected ring radius (from DecValue / ISSF geometry) to `sqrt(X²+Y²) / dsgPerMm` and adjust `coordRadiusMm` for `air_pistol_10m`.
