<script lang="ts">
	// Every technical term the app shows, explained once, in one place (#604).
	// The views themselves lead with plain language and keep the abbreviation
	// as a small aside; this page is for the moment someone wants to know what
	// the aside actually means — or looks the term up elsewhere and needs to
	// connect it back.
	//
	// Content, not logic: it lives here rather than behind an endpoint because
	// nothing computes it and nothing else consumes it.
	const entries = [
		{
			term: 'Fitness',
			jargon: 'CTL, Chronic Training Load',
			color: 'var(--chart-ctl)',
			text: `Wie viel du in den letzten sechs Wochen insgesamt trainiert hast. Steigt langsam und fällt
				langsam — deshalb ist das die Zahl, die deine Grundlage beschreibt. Ein einzelnes hartes
				Wochenende bewegt sie kaum, sechs Wochen Pause dagegen deutlich.`
		},
		{
			term: 'Müdigkeit',
			jargon: 'ATL, Acute Training Load',
			color: 'var(--chart-atl)',
			text: `Wie viel du in der letzten Woche trainiert hast. Reagiert schnell: eine harte Ausfahrt
				treibt sie sofort hoch, zwei ruhige Tage bringen sie wieder runter.`
		},
		{
			term: 'Frische',
			jargon: 'TSB, Training Stress Balance',
			color: 'var(--chart-tsb)',
			text: `Fitness minus Müdigkeit. Über null bist du erholt und kannst zugreifen, unter null steckst
				du in einer Belastungsphase. Dauerhaft weit im Plus heißt allerdings auch, dass du gerade
				Fitness abbaust.`
		},
		{
			term: 'Belastung',
			jargon: 'TSS, Training Stress Score',
			color: 'var(--color-brand)',
			text: `Wie viel eine einzelne Fahrt dich gekostet hat — Dauer und Intensität zusammengerechnet.
				100 entspricht einer Stunde an deiner Schwelle. Eine lange lockere Fahrt kann denselben Wert
				ergeben wie eine kurze harte.`
		},
		{
			term: 'Intensität',
			jargon: 'IF, Intensity Factor',
			color: 'var(--chart-power)',
			text: `Wie hart eine Fahrt im Verhältnis zu deiner eigenen Schwelle war. 100 % heißt: so hart, wie
				du es ungefähr eine Stunde durchhältst. Unter 75 % ist Grundlagentempo, über 95 % Wettkampf.`
		},
		{
			term: 'Schwellenleistung',
			jargon: 'FTP, Functional Threshold Power',
			color: 'var(--chart-power)',
			text: `Die Leistung in Watt, die du ungefähr eine Stunde am Stück treten kannst. Sie ist der
				Bezugspunkt für fast alles andere: ohne sie kann die App nicht sagen, ob eine Fahrt für dich
				hart war. Braucht einen Leistungsmesser — die App schätzt sie dir aus deinen Fahrten.`
		},
		{
			term: 'Schwellenpuls',
			jargon: 'LTHR, Lactate Threshold Heart Rate',
			color: 'var(--chart-heart-rate)',
			text: `Dasselbe wie die Schwellenleistung, nur über den Puls: der Puls, den du etwa eine Stunde
				halten kannst. Der Ersatzweg, wenn kein Leistungsmesser am Rad ist — etwas ungenauer, weil
				Puls von Hitze, Schlaf und Kaffee mitbestimmt wird.`
		},
		{
			term: 'Normalisierte Leistung',
			jargon: 'NP, Normalized Power',
			color: 'var(--chart-power)',
			text: `Ein Durchschnitt, der harte Abschnitte stärker gewichtet als Rollpassagen. Er bildet
				besser ab, wie anstrengend eine Fahrt war, als der schlichte Wattschnitt: 200 Watt gleichmäßig
				sind leichter als ständiger Wechsel zwischen 100 und 300.`
		},
		{
			term: 'Kletterrate',
			jargon: 'VAM, Velocità Ascensionale Media',
			color: 'var(--chart-elevation)',
			text: `Wie viele Höhenmeter du pro Stunde schaffst. Braucht keinerlei Messgerät — nur Höhe und
				Zeit — und ist zwischen Fahrten direkt vergleichbar. Mit deinem Gewicht lässt sich daraus
				grob schätzen, wie viel Kraft du am Berg getreten hast.`
		},
		{
			term: 'Gegen- und Rückenwind',
			jargon: null,
			color: 'var(--color-info)',
			text: `Die App rechnet den Wind auf deine tatsächliche Fahrtrichtung um, Abschnitt für Abschnitt.
				Deshalb steht dort „auf 50 % der Strecke gegen den Wind" statt eines Mittelwerts — bei einer
				Hin-und-zurück-Runde hebt sich der nämlich fast auf, obwohl der Wind die Fahrt geprägt hat.`
		}
	];
</script>

<h1>Was die Begriffe bedeuten</h1>
<p class="intro">
	Die App zeigt dir überall zuerst, was etwas bedeutet, und die Fachbezeichnung nur klein daneben.
	Hier stehen sie alle — falls du eine davon woanders liest oder genauer wissen willst, was dahinter
	steckt.
</p>

<dl class="glossary">
	{#each entries as entry (entry.term)}
		<div class="entry" style="--chip-color: {entry.color}">
			<dt>
				{entry.term}
				{#if entry.jargon}<span class="chip">{entry.jargon}</span>{/if}
			</dt>
			<dd>{entry.text}</dd>
		</div>
	{/each}
</dl>

<style>
	.intro {
		color: var(--color-text-muted);
		max-width: 60ch;
		margin-bottom: 1.5rem;
	}

	.glossary {
		display: grid;
		gap: 0.75rem;
		grid-template-columns: repeat(auto-fit, minmax(20rem, 1fr));
		align-items: start;
		margin: 0;
	}

	.entry {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-sm);
		padding: 1.25rem;
	}

	dt {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		font-size: var(--text-lg);
		font-weight: 700;
		margin-bottom: 0.5rem;
	}

	dd {
		margin: 0;
		line-height: 1.5;
	}
</style>
