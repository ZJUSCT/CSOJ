"use client";

import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { Timer } from "lucide-react";

interface RefreshIntervalSelectorProps {
    onIntervalChange: (interval: number) => void;
    defaultValue?: number;
}

const intervalOptions = [
    { label: "Off", value: 0 },
    { label: "1 second", value: 1000 },
    { label: "5 seconds", value: 5000 },
    { label: "15 seconds", value: 15000 },
    { label: "30 seconds", value: 30000 },
];

export function RefreshIntervalSelector({ onIntervalChange, defaultValue = 1000 }: RefreshIntervalSelectorProps) {
    return (
        <div className="flex items-center gap-2">
            <Timer className="h-4 w-4 text-muted-foreground" />
            <Label htmlFor="refresh-interval" className="text-sm text-muted-foreground shrink-0">
                Refresh
            </Label>
            <Select
                defaultValue={String(defaultValue)}
                onValueChange={(value) => onIntervalChange(Number(value))}
            >
                <SelectTrigger id="refresh-interval" className="w-[140px] h-9">
                    <SelectValue placeholder="Select interval" />
                </SelectTrigger>
                <SelectContent>
                    {intervalOptions.map(option => (
                        <SelectItem key={option.value} value={String(option.value)}>
                            {option.label}
                        </SelectItem>
                    ))}
                </SelectContent>
            </Select>
        </div>
    );
}