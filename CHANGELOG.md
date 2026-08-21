# Änderungsnotizen

## Seit v0.1

- **UDP-Weiterleitung** — optionales `udpForward` (`Host:Port`, oder nur Port für `127.0.0.1`) sendet jedes OpticScore-Datagramm unverändert an ein zweites Dashboard. Für eine zweite Instanz auf demselben Rechner oder über eine Netzwerkgrenze. Nach Änderung Neustart nötig.
- **Inaktive Bahnen** — jede Instanz kann Bahnen abwählen, die sie nicht anzeigt (`runtime.xml` / Menü). Diese Bahnen sind ausgeblendet, Schüsse darauf werden hier verworfen; eine Weiterleitung erreicht trotzdem das nächste Dashboard.
- **Probeserien** — Einschießen-Serien stehen im Footer; das Probe-Dreieck bleibt, wenn man sie anschaut.
- **Ring-Reader-QR** — Serien-IDs enthalten die Uhrzeit des ersten Schusses (`hh:mm`), damit zwei Starts am selben Tag unterscheidbar bleiben.

Nur Repository: Standprotokolle (`exampledata/`) und `docs/DISAG_QR.md` werden nicht mehr mitgeliefert.
