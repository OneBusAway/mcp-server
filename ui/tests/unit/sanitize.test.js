import { describe, it, expect } from 'vitest';
import { sanitizeHtml } from '../../src/lib/sanitize.js';

describe('sanitizeHtml', () => {
	// XSS payloads — none of these should survive
	it.each([
		['event handlers', '<img src=x onerror=alert(1)>', 'onerror'],
		['script tags', '<script>alert(1)</script>', '<script'],
		['javascript: href', '<a href="javascript:alert(1)">click</a>', 'javascript:'],
		['data: href', '<a href="data:text/html,<script>alert(1)</script>">x</a>', 'data:'],
		['vbscript: href', '<a href="vbscript:msgbox(1)">x</a>', 'vbscript:'],
		['iframe', '<iframe src="https://evil.com"></iframe>', 'iframe'],
		// &#106; is 'j', making &#106;avascript:
		['encoded javascript: href', '<a href="&#106;avascript:alert(1)">x</a>', 'javascript:'],
	])('strips %s', (_, input, forbidden) => {
		expect(sanitizeHtml(input)).not.toContain(forbidden);
	});

	// Attribute stripped but text content must survive
	it.each([
		['on* event handlers', '<p onclick="alert(1)">text</p>', 'onclick'],
		['style attribute', '<span style="color:red">text</span>', 'style='],
		['data-* attributes', '<p data-foo="bar">text</p>', 'data-foo'],
	])('strips %s from allowed tags, preserves text', (_, input, forbidden) => {
		const out = sanitizeHtml(input);
		expect(out).not.toContain(forbidden);
		expect(out).toContain('text');
	});

	it('strips object and embed tags', () => {
		expect(sanitizeHtml('<object data="x.swf"></object>')).not.toContain('object');
		expect(sanitizeHtml('<embed src="x.swf">')).not.toContain('embed');
	});

	// Safe content — must survive
	it.each([
		[
			'mailto links',
			'<a href="mailto:user@example.com">email</a>',
			['href="mailto:user@example.com"'],
		],
		['headings', '<h2>Route 30</h2>', ['<h2>Route 30</h2>']],
		['blockquote', '<blockquote><p>quote</p></blockquote>', ['<blockquote>']],
		[
			'https links',
			'<a href="https://example.com" title="ex">link</a>',
			['href="https://example.com"', 'title="ex"', 'link'],
		],
		[
			'markdown elements',
			'<p><strong>bold</strong> <em>italic</em> <code>inline</code></p>',
			['<strong>bold</strong>', '<em>italic</em>', '<code>inline</code>'],
		],
		['code blocks', '<pre><code>console.log("hi")</code></pre>', ['<pre>', '<code>']],
		['lists', '<ul><li>one</li><li>two</li></ul>', ['<ul>', '<li>one</li>']],
		[
			'tables',
			'<table><thead><tr><th>Route</th></tr></thead><tbody><tr><td>30</td></tr></tbody></table>',
			['<table>', '<th>Route</th>', '<td>30</td>'],
		],
	])('preserves %s', (_, input, expected) => {
		const out = sanitizeHtml(input);
		for (const e of expected) {
			expect(out).toContain(e);
		}
	});
});
