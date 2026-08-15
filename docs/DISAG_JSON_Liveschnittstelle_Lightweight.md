# DISAG JSON Schnittstelle Lightweight
V0.1  06.02.2018  CRO  Erste Fassung
V0.2  08.02.2018  CRO  OSS Einstellungen hinzugefügt
V0.3  14.02.2018  CRO  Series und Result hinzugefügt

Inhaltsverzeichnis
Allgemein, Sinn und Zweck ..................................................................................................................... 1
OUT-JSONInterface.log ........................................................................................................................... 2
UDP Broadcast ......................................................................................................................................... 2
Struktur der JSON Objekte ...................................................................................................................... 3
Message ................................................................................................................................................ 3
Shot ...................................................................................................................................................... 3
Series .................................................................................................................................................... 4
Result .................................................................................................................................................... 4
Shooter ................................................................................................................................................. 4
Club ...................................................................................................................................................... 4
Team ..................................................................................................................................................... 5
MenuItem ............................................................................................................................................. 5
Enumerationen ........................................................................................................................................ 5
MessageVerb ........................................................................................................................................ 5
MessageType ........................................................................................................................................ 5
DiscType ............................................................................................................................................... 6
TrafficLightStatus ................................................................................................................................ 6
Einstellungen in der OSS ......................................................................................................................... 6

Allgemein, Sinn und Zweck
Die JSON Schnittstelle liefert in Echtzeit Informationen über die aktuelle Nutzung einer OpticScore-
Anlage. Sie wird aus der OpticScoreServer-Software heraus bereitgestellt. Zudem werden Endgeräte

---

über die Schnittstelle gesteuert, wenn eine parallele, gleichzeitige Ausführung von Befehlen auf
mehreren Geräten notwendig ist.

OUT-JSONInterface.log
Die Log/Text-Datei wird unter %ProgramData%\DisagOpticScore erzeugt kontinuierlich beschrieben.
Überschreitet die Datei eine Größe von 5MB, wird die aktuelle Datei nach OUT-JSONInterface.log.1
verschoben und OUT-JSONInterface.log weiter beschrieben. Es werden maximal zwei Dateien
gehalten. In die Log-Datei werden nur Messages vom Typ „Event“ geschrieben.
Beim Öffnen der Datei muss darauf geachtet werden, dass diese nicht im exklusiven Zugriff geöffnet
wird, da die OSS die Datei sonst nicht befüllen kann.

UDP Broadcast
Alle Informationen werden parallel zur Logdatei über einem UDP Broadcast auf Port 30169 im lokalen
Netzwerk verteilt. Mit einem Client können die Daten auf diesem Port abgegriffen werden.
Beispiel-Client unter Java:

---

Struktur der JSON Objekte
Alle Objekte besitzen das Attribut „UUID“. Dabei handelt es sich um eine UUIDv4 (uppercase), z. B.:
6727B7C5-D772-4419-ABCC-A84098F3C91C.

Message
Parameter  Typ  Beschreibung  Werte
MessageType  enum  MessageType
MessageVerb  enum  MessageVerb
Ranges  Command:  Liste der Stände, die von einem Kommando
Array<int>  betroffen sind oder Standnummer von dem ein
Event: int  Event ausgelöst wurde
Sequential  bool  Wenn mehrere Kommandos in Objects vorhanden
sind, bedeutet Sequential = true, dass diese
Befehle sequentiell abgearbeitet werden müssen
Objects  Array  z. B. Liste von Shot

Shot
Parameter  Typ  Beschreibung  Werte
ShotDateTime  DateTime  Zeitstempel des Schusses  yyyy-MM-dd HH:mm:ss.fff
TLStatus  enum  Zustand der Ampel  off, red, green
während des Schusses,
TrafficLightStatus
LastTLChange  int  Zeit in ms seit letzter  >= 0
Ampelschaltung
Source  enum  Herkunft des Schusses;  OpticScore, RedDot
OpticScore = Messrahmen
Range  int  Standnummer  >= 0
Shooter  object  Shooter
DiscType  enum  DiscType  LG, LGA, LP, LPA, KK, KKA, KYFFH,
ZS, ZSA, ZSTRD, LPS, LPI
X  int  X-Position des Schusses  -9000 < X < 9000
(vom Zentrum des
Messbereichs aus)
Y  int  Y-Position des Schusses  -9000 < Y < 9000
Distance  float  Teiler  0 <= Distance <= 12700
Count  int  Nummer des Schusses  >= 0
FullValue  int  Ringwert ohne Zehntel  0 <= FullValue <= 10
DecValue  float  Ringwert mit Zehntel  0 <= DecValue <= 10.9

---

Run  int  Durchgang innerhalb der  >= 0
Disziplin
IsValid  bool  Schuss gültig  true, false
IsWarmup  bool  Probeschuss  true, false
IsHot  bool  Wertungsschuss  true, false
IsDummy  bool  Generierter Schuss  true, false
IsInnerten  bool  Innenzehner  true, false
IsShootoff  bool  Stechschuss  true, false
MenuItem  object  MenuItem
Remark  string  Hinweis, z. B. Ringabzug

Series
Parameter  Typ  Beschreibung  Werte
Shooter  object  Shooter
ID  int  Nummer der Serie  ID > 0
FullValue  int  Serienwert ohne Zehntel
DecimalValue  float  Serienwert mit Zehntel

Result
Parameter  Typ  Beschreibung  Werte
Shooter  object  Shooter
FullValue  int  Ergebnis ohne Zehntel  FullValue >= 0 <= (ShotCount * 10)
DecimalValue  float  Ergebnis mit Zehntel  DecValue >= 0 <= (ShotCount * 10.9)

Shooter
Parameter  Typ  Beschreibung  Werte
Firstname  string  Vorname
Lastname  string  Nachname
Birthyear  int  Geburtsjahr  >1900
InternalID  string  Interne ID
Identification  string  Passnummer
Team  object  Team  Kann NULL sein
Club  object  Club  Kann NULL sein

Club
Parameter  Typ  Beschreibung  Werte

---

Name  string  Vereinsname
ShortName  string  Vereinsname (kurz)
ID  string  Vereinsnummer

Team
Parameter  Typ  Beschreibung  Werte
Name  string  Mannschaftsname
ShortName  string  Mannschaftsname (kurz)

MenuItem
Parameter  Typ  Beschreibung  Werte
MenuID  string  ID des Menüeintrags  z. B. 100_4
MenuPointName  string  Name des Menüpunkts  z. B. „Neue Schießzeiten“
MenuItemName  string  Name des Eintrags  z. B. „LG 40 Schuss“

Enumerationen
MessageVerb
Wert  Command  Event
Various  Wenn mehrere verschiedene Befehle
in Objects vorhanden hat jeder davon
hat den Parameter CommandType
(entspricht MessageType) für sich
gesetzt.
Shot    Schuss gefallen
Series    Serie fertig
Result    Gesamtergebnis fertig

MessageType
Wert  Beschreibung
Command  Der Empfänger der Message muss prüfen, ob sie für ihn von Bedeutung ist
(anhand der Ranges) und ggf. ausführen
Event  Die Message enthält Informationen wie Schüsse, Standbelegung, etc. Das
Attribut Sequential ist uninteressant. Pro Event-Message ist genau ein Objekt
in Objects
Bei Events werden nur die Werte in den Objekten gesetzt, die sich geändert
haben, z. B. wenn Zehntelschuss an einem Stand aktiviert wurde, werden
nicht alle Einstellungen geliefert

---

DiscType
Wert  Beschreibung
LG  Luftgewehr
LGA  Luftgewehr-Auflage
LP  Luftpistole
LPA  Luftpistole-Auflage
KK  Kleinkalibergewehr
KKA  Kleinkalibergewehr-Auflage
ZS  Zimmerstutzen
ZSA  Zimmerstutzen-Auflage
ZSTRD  Zimmerstutzen-Traditionell
KYFFH  Kyffhäuserscheibe
LPS  DE Luftpistole Schnellfeuer
LPI  IT Luftpistole

TrafficLightStatus
Wert  Beschreibung
off  Ampel ist aus
red  Ampel ist rot
green  Ampel ist grün

Einstellungen in der OSS
Die Einstellungen der JSON Schnittstelle können in der OSS unter Extras -> Optionen -> „JSON Live“
vorgenommen werden.