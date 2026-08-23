# Änderungsnotizen

## v0.2

- **Zehn Trainings-Spiele** — Tauziehen, Kettenreaktion, Biathlon, Schrumpfender Kreis, Kronen-Duell, Bank oder Risiko, KO-Pokal, Schießgolf, Turmbau und Ansage-Duell. Gewertet wird nur ein höherer Ringwert oder ein Loch-im-Loch (50 % Überdeckung, Rohwert über 8,5). Gemischte Leistungsgruppen gleichen über persönlichen Schnitt oder Handicap aus, nie über andere Zielaufgaben.
- **UDP-Weiterleitung** — optionales `udpForward` (`Host:Port`, oder nur Port für `127.0.0.1`) sendet jedes OpticScore-Datagramm unverändert an ein zweites Dashboard. Für eine zweite Instanz auf demselben Rechner oder über eine Netzwerkgrenze. Nach Änderung Neustart nötig.
- **Inaktive Bahnen** — jede Instanz kann Bahnen abwählen, die sie nicht anzeigt (`runtime.xml` / Menü). Diese Bahnen sind ausgeblendet, Schüsse darauf werden hier verworfen; eine Weiterleitung erreicht trotzdem das nächste Dashboard.
- **Bahnwahl in Spielen** — die Mitgliedschaft aus dem Seitenmenü gilt in jedem Spiel; die Wertung startet mit dem ersten Wettkampfschuss.
- **Probeserien** — Einschießen-Serien stehen im Footer; das Probe-Dreieck bleibt, wenn man sie anschaut.
- **Ring-Reader-QR** — Serien-IDs enthalten die Uhrzeit des ersten Schusses (`hh:mm`), damit zwei Starts am selben Tag unterscheidbar bleiben.
- **Zehner-Bingo** — Karte 8,5–10,9, gemischt beim Start; Clips für Glas-Treffer und Bingo-Gewinn.

Nur Repository: Standprotokolle (`exampledata/`) und `docs/DISAG_QR.md` werden nicht mehr mitgeliefert.
