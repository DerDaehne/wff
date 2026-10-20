// Vendors Rekta Design (ADRs adr-002-wff-rekta-design, adr-004-wff-rekta-js)
// into src/lib/rekta/: rekta.tokens.css, rekta.fonts.css, rekta.css, rekta.js and
// rekta.strings.de.js, with their TypeScript declarations, byte-for-byte below a
// provenance header, plus the Barlow woff2 files rekta.fonts.css points at.
// No npm dependency on purpose: a dependency would change the Nix
// fixed-output hash on every Rekta bump.
//
//   node scripts/sync-rekta.mjs ~/Projekte/rekta-design   # build that clone's HEAD and copy
//   node scripts/sync-rekta.mjs --verify                  # vendored files still match their header hash
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFileSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const OUT = new URL('../src/lib/rekta/', import.meta.url).pathname;
const FILES = [
	'rekta.tokens.css',
	'rekta.fonts.css',
	'rekta.css',
	'rekta.js',
	'rekta.strings.de.js',
	'rekta.d.ts',
	'rekta.strings.de.d.ts'
];
const HASH_LINE = /^ \* SHA-256 of everything below this header: ([0-9a-f]{64})$/m;

const sha256 = (s) => createHash('sha256').update(s).digest('hex');

function verify() {
	for (const name of FILES) {
		const text = readFileSync(join(OUT, name), 'utf8');
		const end = text.indexOf('*/\n') + 3;
		const expected = text.slice(0, end).match(HASH_LINE)?.[1];
		if (!expected) throw new Error(`${name}: no provenance header`);
		if (sha256(text.slice(end)) !== expected) {
			throw new Error(`${name} was edited after vendoring; re-run sync-rekta.mjs instead`);
		}
	}
	console.log(`rekta: ${FILES.length} vendored files match their headers`);
}

function sync(repo) {
	const git = (...args) =>
		execFileSync('git', ['-C', repo, ...args], {
			encoding: 'utf8',
			stdio: ['ignore', 'pipe', 'ignore']
		}).trim();
	if (git('status', '--porcelain'))
		throw new Error(`${repo} has uncommitted changes; the header would name the wrong commit`);
	const commit = git('rev-parse', 'HEAD');
	let tag = '';
	try {
		tag = git('describe', '--tags', '--exact-match');
	} catch {
		// No tag on this commit: the commit is the pin.
	}

	// --check first: it fails on a contrast or token regression, and then nothing is copied.
	execFileSync('node', ['scripts/build.mjs', '--check'], { cwd: repo, stdio: 'inherit' });
	execFileSync('node', ['scripts/build.mjs'], { cwd: repo, stdio: 'inherit' });
	// WFF's heart-rate zone colours (app.css) against this Rekta's rules for a domain colour code.
	execFileSync(
		'node',
		['scripts/build.mjs', '--check-product', new URL('../src/app.css', import.meta.url).pathname],
		{ cwd: repo, stdio: 'inherit' }
	);

	for (const name of FILES) {
		const body = readFileSync(join(repo, 'dist', name), 'utf8');
		const header = [
			'/*! Rekta Design, vendored by WFF. Do not edit: run frontend/scripts/sync-rekta.mjs.',
			' * Source: https://github.com/DerDaehne/rekta-design',
			` * Commit: ${commit}${tag ? `\n * Tag: ${tag}` : ''}`,
			` * Generator: scripts/build.mjs -> dist/${name}`,
			` * SHA-256 of everything below this header: ${sha256(body)}`,
			' * License: MIT (code, tokens, CSS, JS), see LICENSE and NOTICE in the source repository;',
			' * some token values derive from the Porsche Design System (Apache-2.0, values only).',
			' * Barlow: SIL Open Font License 1.1, see fonts/OFL.txt.',
			' */',
			''
		].join('\n');
		writeFileSync(join(OUT, name), header + body);
	}

	mkdirSync(join(OUT, 'fonts'), { recursive: true });
	for (const font of readdirSync(join(repo, 'dist', 'fonts'))) {
		copyFileSync(join(repo, 'dist', 'fonts', font), join(OUT, 'fonts', font));
	}
	console.log(`rekta: vendored ${commit}${tag ? ` (${tag})` : ''}`);
}

const arg = process.argv[2];
if (arg === '--verify') verify();
else if (arg) sync(arg);
else throw new Error('usage: sync-rekta.mjs <path to a rekta-design clone> | --verify');
