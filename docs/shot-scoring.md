# Shot attributes (OpticScore / ISSF)

Every shot has four fields. They are not independent ring tables.

1. **Coordinates** `X`, `Y` — hole centre. **100 DSG = 1 mm**. OpticScore range is about ±9000 (±90 mm).
2. **Teiler** (`Distance`) — `hypot(X, Y)` in DSG. Same unit as the coordinates.
3. **DecValue** — decimal ring from Teiler (ISSF 6.3.3.1: each full ring split into ten equal tenths; **10.9** is innermost).
4. **IntValue** (`FullValue`) — DecValue with the tenth stripped, **no rounding**: `floor(DecValue)`.  
   10.0…10.9 → 10, 9.9 → 9, 0.0 → 0.

`IsInnerten` is a fifth flag. It is **not** IntValue and **not** “hit the printed 10-dot”.

Checked against `exampledata/` OpticScore logs (LG ~8900 shots, LP 28 unique, KK series in `KK-10-Schuss.txt`).

## Teiler → DecValue

Same formula for every discipline; only **step** changes.

```
tenths   = floor(Teiler / step)     # Teiler 0 counts as 10.9
DecValue = max(0.0, 10.9 − tenths × 0.1)
IntValue = floor(DecValue)
```

Touching a limit counts as the **higher** tenth. OpticScore may keep that higher tenth a fraction past the exact multiple (e.g. LG 25.8 still 10.9).

| Discipline | Step (DSG / 0.1) | Why |
|------------|------------------|-----|
| **LG** (10 m air rifle) | **25** | ISSF ring pitch 5 mm diameter → 2.5 mm radius / 10 |
| **LP** (10 m air pistol) | **80** | ISSF ring pitch 16 mm diameter → 8 mm radius / 10 |
| **KK** (50 m rifle) | **80** | ISSF ring pitch 16 mm diameter → 8 mm radius / 10 |

The printed 10-ring is **not** DecValue 10.0:

- LG 0.5 mm dot (Teiler 25) = **10.9**
- LP 11.5 mm ring (Teiler 575) ≈ **10.2**
- LG/LP **10.0** is Teiler 250 / 800

## Outer Teiler for each tenth (`≤`)

**LG (step 25)**

| Dec | Teiler | Dec | Teiler |
|-----|--------|-----|--------|
| 10.9 | 25 | 9.9 | 275 |
| 10.8 | 50 | 9.0 | 500 |
| 10.7 | 75 | 8.0 | 750 |
| 10.6 | 100 | 7.0 | 1000 |
| 10.5 | 125 | 6.0 | 1250 |
| 10.4 | 150 | 5.0 | 1500 |
| 10.3 | 175 | 4.0 | 1750 |
| 10.2 | 200 | 3.0 | 2000 |
| 10.1 | 225 | 2.0 | 2250 |
| 10.0 | 250 | 1.0 | 2500 |
| | | 0.0 | > 2500 |

**LP and KK (step 80)**

| Dec | Teiler | Dec | Teiler |
|-----|--------|-----|--------|
| 10.9 | 80 | 9.9 | 880 |
| 10.8 | 160 | 9.0 | 1600 |
| 10.7 | 240 | 8.0 | 2400 |
| 10.6 | 320 | 7.0 | 3200 |
| 10.5 | 400 | 6.0 | 4000 |
| 10.4 | 480 | 5.0 | 4800 |
| 10.3 | 560 | 4.0 | 5600 |
| 10.2 | 640 | 3.0 | 6400 |
| 10.1 | 720 | 2.0 | 7200 |
| 10.0 | 800 | 1.0 | 8000 |
| | | 0.0 | > 8000 |

From 9.9 down, add one step per tenth (LG +25, LP/KK +80).

UDP consistency checks live in `udp/validate.go` (`rifleBandDSG = 25`, `pistolBandDSG = 80`).
