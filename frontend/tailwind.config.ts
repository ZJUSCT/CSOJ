import type { Config } from "tailwindcss"

const TAG_COLORS = [
  "bg-blue-100", "text-blue-800", "dark:bg-blue-900", "dark:text-blue-200",
  "bg-green-100", "text-green-800", "dark:bg-green-900", "dark:text-green-200",
  "bg-red-100", "text-red-800", "dark:bg-red-900", "dark:text-red-200",
  "bg-purple-100", "text-purple-800", "dark:bg-purple-900", "dark:text-purple-200",
  "bg-pink-100", "text-pink-800", "dark:bg-pink-900", "dark:text-pink-200",
  "bg-indigo-100", "text-indigo-800", "dark:bg-indigo-900", "dark:text-indigo-200",
  "bg-cyan-100", "text-cyan-800", "dark:bg-cyan-900", "dark:text-cyan-200",
  "bg-orange-100", "text-orange-800", "dark:bg-orange-900", "dark:text-orange-200",
  "bg-teal-100", "text-teal-800", "dark:bg-teal-900", "dark:text-teal-200",
  "bg-amber-100", "text-amber-800", "dark:bg-amber-900", "dark:text-amber-200",
];
const colorSafelist = TAG_COLORS.flatMap(group => group.split(" "));


const config = {
  darkMode: ["class"],
  content: [
	'./pages/**/*.{ts,tsx}',
	'./components/**/*.{ts,tsx}',
	'./app/**/*.{ts,tsx}',
	'./src/**/*.{ts,tsx}',
	],
  safelist: [
	...colorSafelist,
  ],
  prefix: "",
  theme: {
	container: {
		center: true,
		padding: '2rem',
		screens: {
			'2xl': '1400px'
		}
	},
	extend: {
		fontFamily: {
			sans: [
				'var(--font-sans)'
			]
		},
		colors: {
			border: 'hsl(var(--border))',
			input: 'hsl(var(--input))',
			ring: 'hsl(var(--ring))',
			background: 'hsl(var(--background))',
			foreground: 'hsl(var(--foreground))',
			primary: {
				DEFAULT: 'hsl(var(--primary))',
				foreground: 'hsl(var(--primary-foreground))'
			},
			secondary: {
				DEFAULT: 'hsl(var(--secondary))',
				foreground: 'hsl(var(--secondary-foreground))'
			},
			destructive: {
				DEFAULT: 'hsl(var(--destructive))',
				foreground: 'hsl(var(--destructive-foreground))'
			},
			muted: {
				DEFAULT: 'hsl(var(--muted))',
				foreground: 'hsl(var(--muted-foreground))'
			},
			accent: {
				DEFAULT: 'hsl(var(--accent))',
				foreground: 'hsl(var(--accent-foreground))'
			},
			popover: {
				DEFAULT: 'hsl(var(--popover))',
				foreground: 'hsl(var(--popover-foreground))'
			},
			card: {
				DEFAULT: 'hsl(var(--card))',
				foreground: 'hsl(var(--card-foreground))'
			},
			chart: {
				'1': 'hsl(var(--chart-1))',
				'2': 'hsl(var(--chart-2))',
				'3': 'hsl(var(--chart-3))',
				'4': 'hsl(var(--chart-4))',
				'5': 'hsl(var(--chart-5))'
			}
		},
		borderRadius: {
			lg: 'var(--radius)',
			md: 'calc(var(--radius) - 2px)',
			sm: 'calc(var(--radius) - 4px)'
		},
		keyframes: {
			'accordion-down': {
				from: {
					height: '0'
				},
				to: {
					height: 'var(--radix-accordion-content-height)'
				}
			},
			'accordion-up': {
				from: {
					height: 'var(--radix-accordion-content-height)'
				},
				to: {
					height: '0'
				}
			}
		},
		animation: {
			'accordion-down': 'accordion-down 0.2s ease-out',
			'accordion-up': 'accordion-up 0.2s ease-out'
		},
		// Tight Markdown Theme
		typography: ({ theme }: { theme: any }) => ({
			tight: {
				css: {
					'--tw-prose-body': {
						lineHeight: theme('lineHeight.normal'),
					},
					p: {
					'margin-top': theme('spacing.3'),
					'margin-bottom': theme('spacing.3'),
					},
					ul: {
					'margin-top': theme('spacing.3'),
					'margin-bottom': theme('spacing.3'),
					},
					ol: {
					'margin-top': theme('spacing.3'),
					'margin-bottom': theme('spacing.3'),
					},
					'li': {
						'margin-top': theme('spacing.1'),
						'margin-bottom': theme('spacing.1'),
					},
					'h2': {
						'margin-top': theme('spacing.3'),
						'margin-bottom': theme('spacing.3'),
					},
					'h3': {
						'margin-top': theme('spacing.2'),
						'margin-bottom': theme('spacing.2'),
					},
					blockquote: {
						'margin-top': theme('spacing.4'),
						'margin-bottom': theme('spacing.4'),
					},
				},
			},
		}),
	}
  },
  plugins: [
	require("tailwindcss-animate"),
	require('@tailwindcss/typography'),
  ],
} satisfies Config

export default config