# Änderungsnotizen

## v0.3

- **Classic Range Condensed** — knappe Hallenansicht: Kopfzeile, Scheibe, letzter Schuss, Teiler, Summe und Serien. Ohne Last-10-Diagramm und ohne die große Statstabelle. Im Menü umschalten. Grüße an Daniel.
- **Compact View** — `/compact` (auch Menü **Compact**) zeigt die Masterview ohne Scheibe; Bretter, Bäume und Karten nutzen den freien Platz.
- **Schussfarben** — im Seitenmenü Farbsättigung plus Farbfelder: gedämpfter Regenbogen (Standard) oder eine Farbe von hell nach dunkel.
- **Wiederherstellung** — unter Einstellungen Konsolenlog (`log/<Startzeit>.txt`) oder OpticScore-JSON einfügen; Schüsse laufen durch die normale Prüfung, das aktive Spiel wertet mit. Zufallsereignisse (z. B. Reifenplatzer) stehen nicht im Log.
- **Admin- und Public-Port** — Admin (Standard 8080) steuert alles; optionaler `publicPort` ist nur lesender Masterview und Stände, ohne `/config`. Damit kann man die Webseiten auch in weniger vertrauenswürdige Netze rein routen.
- **UDP-Prüfung** — Schüsse, bei denen Lage, Teiler und Ringwert nicht zusammenpassen, kommen nicht in den Live-Stand.
- **Plugins als Ordner** — mitgelieferte Spiele liegen unter `plugins/{id}/`, nicht mehr als `.srplugin.zip`. Zusätzliche Plugins weiter als Ordner (oder einzeln zippen).



## v0.2

- **Zehn Trainings-Spiele** — Tauziehen, Kettenreaktion, Biathlon, Schrumpfender Kreis, Kronen-Duell, Bank oder Risiko, KO-Pokal, Schießgolf, Turmbau und Ansage-Duell. Gewertet wird nur ein höherer Ringwert oder ein Loch-im-Loch (50 % Überdeckung, Rohwert über 8,5). Gemischte Leistungsgruppen gleichen über persönlichen Schnitt oder Handicap aus, nie über andere Zielaufgaben.
- **UDP-Weiterleitung** — optionales `udpForward` (`Host:Port`, oder nur Port für `127.0.0.1`) sendet jedes OpticScore-Datagramm unverändert an ein zweites Dashboard. Für eine zweite Instanz auf demselben Rechner oder über eine Netzwerkgrenze. Nach Änderung Neustart nötig.
- **Inaktive Bahnen** — jede Instanz kann Bahnen abwählen, die sie nicht anzeigt (`runtime.xml` / Menü). Diese Bahnen sind ausgeblendet, Schüsse darauf werden hier verworfen; eine Weiterleitung erreicht trotzdem das nächste Dashboard.
- **Bahnwahl in Spielen** — die Mitgliedschaft aus dem Seitenmenü gilt in jedem Spiel; die Wertung startet mit dem ersten Wettkampfschuss.
- **Probeserien** — Einschießen-Serien stehen im Footer; das Probe-Dreieck bleibt, wenn man sie anschaut.
- **Ring-Reader-QR** — Serien-IDs enthalten die Uhrzeit des ersten Schusses (`hh:mm`), damit zwei Starts am selben Tag unterscheidbar bleiben.
- **Zehner-Bingo** — Karte 8,5–10,9, gemischt beim Start; Clips für Glas-Treffer und Bingo-Gewinn.

Nur Repository: Standprotokolle (`exampledata/`) und `docs/DISAG_QR.md` werden nicht mehr mitgeliefert.