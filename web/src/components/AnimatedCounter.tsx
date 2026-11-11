import { useEffect, useRef, useState } from 'react';

export function AnimatedCounter({ value }: { value: number }) {
  const previous = useRef(0);
  const [display, setDisplay] = useState(() => window.matchMedia('(prefers-reduced-motion: reduce)').matches ? value : 0);
  const [rising, setRising] = useState(false);

  useEffect(() => {
    const start = previous.current;
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches || start === value) {
      previous.current = value;
      setDisplay(value);
      return;
    }

    let frame = 0;
    let flashTimer = 0;
    const beganAt = performance.now();
    const duration = 620;
    if (value > start) {
      setRising(true);
      flashTimer = window.setTimeout(() => setRising(false), 720);
    }

    const tick = (now: number) => {
      const progress = Math.min(1, (now - beganAt) / duration);
      const eased = 1 - Math.pow(1 - progress, 3);
      setDisplay(Math.round(start + (value - start) * eased));
      if (progress < 1) frame = requestAnimationFrame(tick);
      else previous.current = value;
    };
    frame = requestAnimationFrame(tick);
    return () => {
      cancelAnimationFrame(frame);
      window.clearTimeout(flashTimer);
    };
  }, [value]);

  return <span className={`animated-counter${rising ? ' animated-counter--up' : ''}`}>{display.toLocaleString('es-AR')}</span>;
}
