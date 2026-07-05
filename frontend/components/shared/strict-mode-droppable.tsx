"use client";

import { Droppable, DroppableProps } from "react-beautiful-dnd";
import { useEffect, useState } from "react";

// This is a wrapper to make react-beautiful-dnd compatible with React 18 Strict Mode.
export const StrictModeDroppable = ({ children, ...props }: DroppableProps) => {
  const [enabled, setEnabled] = useState(false);

  useEffect(() => {
    const animation = requestAnimationFrame(() => setEnabled(true));
    return () => {
      cancelAnimationFrame(animation);
      setEnabled(false);
    };
  }, []);

  if (!enabled) {
    return null;
  }

  return <Droppable {...props}>{children}</Droppable>;
};