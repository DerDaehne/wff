/*! Rekta Design, vendored by WFF. Do not edit: run frontend/scripts/sync-rekta.mjs.
 * Source: https://github.com/DerDaehne/rekta-design
 * Commit: 1a4133f27c37025f4e6de97db8a8f4a30129a148
 * Tag: v0.5.0
 * Generator: scripts/build.mjs -> dist/rekta.strings.de.js
 * SHA-256 of everything below this header: 2916f297f00fdbdada1508c4f222ea2918a7c31996bef4279e1373547a9be777
 * License: MIT (code, tokens, CSS, JS), see LICENSE and NOTICE in the source repository;
 * some token values derive from the Porsche Design System (Apache-2.0, values only).
 * Barlow: SIL Open Font License 1.1, see fonts/OFL.txt.
 */
// Rekta Design: rekta.js's words in German. Rekta.start({ locale: 'de', strings: de }) uses them; any can be replaced
// on its own. --check keeps the keys the same as the English catalogue in rekta.js.
export const de = {
  undo: 'rückgängig', redo: 'wiederholen', ok: 'ok', cancel: 'abbrechen', live: 'live', since: 'seit', any: 'alle', field: 'Feld',
  noValue: 'kein Wert', reload: 'neu laden',
  undoWindow: { one: (seconds, again) => `, 1 Sekunde${again ? ' zum Rückgängigmachen' : ''}`, other: (seconds, again) => `, ${seconds} Sekunden${again ? ' zum Rückgängigmachen' : ''}` },
  bannerStill: (event, word) => `${event}. Das Banner zeigt weiter „${word}“, bis das endet.`,
  chosen: (summary, chosen) => `${summary}: ${chosen}`,
  selected: { one: (n) => `${n} ausgewählt`, other: (n) => `${n} ausgewählt` },
  columnWidth: (column) => `Breite von ${column}`,
  cellRefused: 'Dieser Wert wurde nicht übernommen.',
  paletteEmpty: (query) => `Nichts zum Hingehen oder Ausführen passt zu „${query}“.`,
  saveFailed: 'Speichern ging nicht. Bitte noch einmal versuchen.',
  guardWhat: 'Dieses Formular',
  guardKicker: 'ungespeichert',
  guardTitle: 'Ohne Speichern verlassen?',
  guardText: { one: (what) => `${what} hat eine ungespeicherte Änderung.`, other: (what, n) => `${what} hat ${n} ungespeicherte Änderungen.` },
  guardDiscard: 'Änderungen verwerfen',
  guardKeep: 'weiter bearbeiten',
  discardTitle: 'Eingaben verwerfen?',
  move: (item) => `${item} verschieben`,
  nothingToUndo: 'Nichts rückgängig zu machen.',
  undone: (what) => `Rückgängig: ${what}`,
  justNow: 'gerade eben',
  secondsAgo: (s) => `vor ${s} s`,
  minutesAgo: (m) => `vor ${m} min`,
  at: (time) => `um ${time}`,
  updated: (ago) => `aktualisiert ${ago}`,
  heldUpdated: (ago) => `angehalten, solange du hinsiehst · aktualisiert ${ago}`,
  staleFor: (s, m, time) => `veraltet · keine Aktualisierung ${s < 60 ? `seit ${s} s` : m < 60 ? `seit ${m} min` : `seit ${time}`}`,
  offlineSince: (ago) => `offline · letzte Aktualisierung ${ago}`,
  updatesWaiting: { one: '1 Aktualisierung wartet · zeigen', other: (n) => `${n} Aktualisierungen warten · zeigen` },
  chartName: (type, series, from, to) => `${type === 'bars' ? 'Balkendiagramm' : 'Liniendiagramm'} von ${series.join(' und ')}, ${from} bis ${to}. Mit den Pfeiltasten liest du jeden Wert; danach folgt eine Tabelle.`,
  chartNumbers: 'die Zahlen zeigen',
  installed: 'Installiert. Es öffnet sich in einem eigenen Fenster.',
  newVersion: 'Eine neue Version ist bereit.',
  noMatch: (query) => `Nichts passt zu „${query}“.`,
  nothingYet: 'Noch nichts zur Auswahl.',
  loadFailed: 'Die Auswahl ließ sich nicht laden. Tippe, um es noch einmal zu versuchen.',
  needAttention: { one: '1 Feld braucht Aufmerksamkeit', other: (n) => `${n} Felder brauchen Aufmerksamkeit` },
  pickedUp: (item, at, of) => `${item} aufgenommen, Position ${at} von ${of}. ↑ ↓ verschiebt, Leertaste legt ab, Escape legt zurück.`,
  putDown: (item, at, of) => `${item} abgelegt auf Position ${at} von ${of}.`,
  putBack: (item) => `${item} zurückgelegt.`,
  position: (item, at, of) => (item ? `${item}: Position ${at} von ${of}.` : `Position ${at} von ${of}`),
};
