/**
 * Microphone permission and device discovery, run **from the console**.
 *
 * The presentation window is the one that actually captures, but it sits behind
 * the console without focus. `getUserMedia` there raises its permission prompt
 * on a window nobody is looking at, so the request appears to do nothing — or
 * gets dismissed and comes back as "Permission denied by user".
 *
 * Permission is granted per origin, not per window. Asking here, where the
 * click happened and the prompt is visible, means the presentation's own call
 * succeeds silently afterwards.
 */
export const useMicrophone = () => {
  const granted = useState('mic-granted', () => false)
  const asking = useState('mic-asking', () => false)
  const problem = useState<string | null>('mic-problem', () => null)

  // VueUse keeps the list live as devices are plugged and unplugged, and only
  // exposes real labels once permission exists.
  const { audioInputs, ensurePermissions, permissionGranted } = useDevicesList({
    constraints: { audio: true, video: false },
    requestPermissions: false,
  })

  const devices = computed(() =>
    audioInputs.value.map(d => ({
      id: d.deviceId,
      label: d.label || 'Micrófono sin nombre',
    })),
  )

  watchEffect(() => {
    if (permissionGranted.value) granted.value = true
  })

  /**
   * Ask for the microphone here, in the focused window. Returns whether it was
   * granted, so the caller can decide what to do next.
   */
  async function request(): Promise<boolean> {
    if (asking.value) return granted.value
    asking.value = true
    problem.value = null
    try {
      // ensurePermissions opens a stream, reads the labels and closes it again;
      // we never hold this one, the presentation opens its own.
      const ok = await ensurePermissions()
      granted.value = ok
      if (!ok) {
        problem.value = 'Chrome no concedió el micrófono. Revisa el candado de la barra de direcciones.'
      }
      return ok
    } catch (err) {
      problem.value = explainMediaError(err)
      granted.value = false
      return false
    } finally {
      asking.value = false
    }
  }

  return { granted, asking, problem, devices, request }
}

/**
 * Browser media errors arrive as English DOMException names that say nothing
 * about what to do next. Translate them into the one sentence that helps.
 */
export function explainMediaError(err: unknown): string {
  const name = (err as DOMException)?.name ?? ''
  const raw = String((err as Error)?.message ?? err)

  if (name === 'NotAllowedError' || /permission denied/i.test(raw)) {
    return 'Cancelaste el permiso del micrófono. Púlsalo otra vez y acepta el aviso de Chrome.'
  }
  if (name === 'NotFoundError') return 'No encuentro ningún micrófono conectado.'
  if (name === 'NotReadableError') {
    return 'El micrófono está ocupado por otra aplicación. Ciérrala y vuelve a intentarlo.'
  }
  if (name === 'OverconstrainedError') {
    return 'Ese micrófono ya no está disponible. Elige otro en la lista.'
  }
  if (name === 'AbortError') return 'La captura se interrumpió antes de empezar.'
  return raw
}
