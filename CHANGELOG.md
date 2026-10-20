# Änderungsnotizen

## v0.5

- **Darts** — Countdown 301, 501 oder 701, Standard 501. Jede Bahn wirft parallel, ein Pfeil pro Schuss. Double-In und Double-Out sind eigene Schalter und standardmäßig aus. Ein Fehl ist 0, Einschießen zählt nicht. Wer zuerst genau 0 erreicht, gewinnt. Unter 0 ist ein Bust, der Rest bleibt; bei Double-Out auch Rest 1 oder genau 0 ohne Double. Die letzten drei Pfeile sind nur zum Ablesen. Liegt die gedruckte Scheibe nicht auf dem Foto-Maß, einmal **Scheibe einmessen** und ein Schuss auf den äußeren Doppel-Draht. Der Bull bleibt die Mitte, alle Ringe skalieren gleich.
- **RWK-Bericht** — im Wettkampf-Dialog von Classic Range Condensed. Die Formulare liegen als PDF in `plugins/classic-range-condensed/` und werden von dort gelesen. Mitgeliefert wird nur **Ohne Kopf**: ohne Briefkopf und ohne die Vereinsliste von Soest-Lippstadt. Die Soest-Lippstadt-Vorlage wird nicht ausgeliefert.
- **Munitionstest** — Auf dem Master werden Lose angelegt, die Tablets wählen sie und können welche ergänzen. Der erste Schuss setzt das Zentrum; alles Weitere wird von diesem Schuss gemessen. X und Y in Millimeter, der Streukreis ist der größte Teiler (100 DSG = 1 mm). In der Regel zehn Schuss, danach ist das Los fertig. Die Waffe steht auf dem Master am Ergebnis. Der Master listet alle Bahnen und Lose und druckt die Gesamtliste. Der Abbruch wächst, sobald der Streukreis ein anderes Los derselben Bahn übertrifft, und noch einmal je weiteres Zehntel.


## v0.4

- **Wettkampf (Classic Range Condensed)** — extra Kachel rechts neben den Bahnen: Mannschaften mit Summe, Prognose und `n/Soll`. Bahnwahl-Häkchen **Wettkampf** (nur CRC). Bahn-Reset löscht die Kachel nicht.
- **Mannschaften** — Zuordnung über UDP-Team oder Verein, Zahnrad auf der Kachel. Mehrere Mannschaften desselben Vereins bekommen eigene Kopffarben; die Bahnköpfe folgen der Mannschaft. Schützen **ohne Mannschaft** bleiben unter ihrem Verein, zählen nicht zur Summe.
- **Analyse** — Plugin zeigt eingefrorene Session-Ergebnisse nacheinander in der Classic-Range-Ansicht. Schließt der Schütze auf derselben Bahn neu, bleibt der abgeschlossene Start wählbar; Zoom, Serien und Scheibe mischen sich nicht mit dem neuen Start.
- **Scheiben-Zoom** — Mausrad zoomt in feinen Schritten auf den Zeiger statt 12 % pro Tick in die Mitte. Ziehen verschiebt den Ausschnitt (Analyse, Classic, Condensed).
- **Hallen-Raster** — 5 aktive Bahnen legen sich 3+2, nicht mehr als eine Spalte winziger Scheiben. Compact blendet Classic/Condensed-Scheiben aus.
- **Disziplin manuell** — Classic und Condensed: auf das Disziplin-Label tippen, Liste Automatisch / LG / LG Auflage / LP / LP Auflage / KK. Korrigiert falsche Programme auf der Bahn (Scheibe, Ganz/Dezimal). Gilt bis Schütze- oder Programmwechsel. Das OpticScore-Label bleibt, wenn die Familie schon stimmt (`LG 40 Schuss` bleibt `LG 40 Schuss`).
- **Scheibe pro Disziplin** — Einstellungen mappt LG / LP / KK auf Scheibenprofile. Die alten pro-Bahn-Scheiben entfallen.
- **Programmlänge** — Schusszahl aus OpticScore (`LG 20/40 Schuss`, LGA 30, LP 40, LPA 30, KK `3x20` / `3x40` / `20/20/20`). `unbegrenzt` zählt in der Halle als 100.
- **HR** — Condensed-Footer: Hochrechnung statt des langen Prognose-Worts.
- **Tannebaum** — ein Treffer fällt zuerst die präziseste noch erreichbare eigene Nadel (auch kleinere offene auf derselben Stufe). Geschenk erst, wenn der eigene Baum darunter nichts mehr braucht.
- **Tablet View Footer**— Spiel-Legenden in der Fußzeile auf deckendem Grund, damit der Text über der Szene lesbar bleibt.



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