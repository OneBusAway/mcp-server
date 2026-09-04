import DOMPurify from 'dompurify';

const PURIFY_CONFIG = {
	ALLOWED_TAGS: [
		'p',
		'br',
		'strong',
		'em',
		'b',
		'i',
		'code',
		'pre',
		'ul',
		'ol',
		'li',
		'blockquote',
		'h1',
		'h2',
		'h3',
		'h4',
		'h5',
		'h6',
		'a',
		'hr',
		'table',
		'thead',
		'tbody',
		'tr',
		'th',
		'td',
		'del',
		's',
		'span',
	],
	ALLOWED_ATTR: ['href', 'title', 'class'],
	ALLOW_DATA_ATTR: false,
	ALLOWED_URI_REGEXP: /^(https:|mailto:)/i,
};

export function sanitizeHtml(html) {
	return DOMPurify.sanitize(html, PURIFY_CONFIG);
}
