export default defineAppConfig({
  ui: {
    // Colour belongs to the voices and to the playhead. Everything the
    // interface itself says is grey, so a coloured pixel always means a
    // character is involved.
    colors: { primary: 'zinc', neutral: 'zinc' },
    button: { defaultVariants: { color: 'neutral' } },
  },
})
