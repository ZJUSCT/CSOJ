"use client";

import React, { useMemo } from 'react';
import ReactECharts from 'echarts-for-react';
import type { EChartsOption, SeriesOption } from 'echarts';
import { useTheme } from 'next-themes';
import { format } from 'date-fns';
import { TrendEntry } from '@/lib/types';

interface EchartsTrendChartProps {
    trendData: TrendEntry[];
    contestStartTime: string | number | Date;
    contestEndTime: string | number | Date;
}

const EchartsTrendChart: React.FC<EchartsTrendChartProps> = ({
    trendData,
    contestStartTime,
    contestEndTime,
}) => {
    const { theme } = useTheme();

    const userSeriesData: SeriesOption[] = useMemo(() => {
        if (!trendData || trendData.length === 0) {
            return [];
        }
        
        const currentTime = new Date();
        const contestEndDate = new Date(contestEndTime);
        const lastDataPointTime = contestEndDate.getTime() < currentTime.getTime() ? contestEndDate : currentTime;

        return trendData.map((user): SeriesOption => {
            const lastHistoryItem = user.history.length > 0 ? user.history[user.history.length - 1] : null;
            const lastScore = lastHistoryItem ? lastHistoryItem.score : 0;

            return {
                name: user.nickname,
                type: 'line',
                step: 'end',
                symbol: 'none',
                data: [
                    [new Date(contestStartTime).getTime(), 0],
                    ...user.history.map(p => [new Date(p.time).getTime(), p.score]),
                    [lastDataPointTime.getTime(), lastScore],
                ],
            };
        });
        
    }, [trendData, contestStartTime, contestEndTime]);

    const chartOptions: EChartsOption = useMemo(() => {
        const isDark = theme === 'dark';
        const labelColor = isDark ? '#ccc' : '#333';
        const lineColor = isDark ? '#555' : '#eee';
        const backgroundColor = isDark ? 'rgba(31, 41, 55, 0.9)' : 'rgba(255, 255, 255, 0.9)';
        
        const startTime = new Date(contestStartTime).getTime();
        const endTime = new Date(contestEndTime).getTime();
        const nowTime = new Date().getTime();
        const isFinished = nowTime > endTime;

        let xAxisMax;
        if (isFinished) {
            xAxisMax = endTime;
        } else {
            const durationSoFar = nowTime - startTime;
            const buffer = Math.max(durationSoFar * 0.1, 60000); 
            xAxisMax = nowTime + buffer;
        }

        const absoluteMaxScore = trendData.reduce((maxScore, user) => {
            const userMaxScore = user.history.reduce((max, point) => Math.max(max, point.score), 0);
            return Math.max(maxScore, userMaxScore);
        }, 0);

        const markLineData = [];
        
        if (!isFinished) {
            markLineData.push({
                xAxis: nowTime,
                lineStyle: {
                    color: isDark ? '#4ade80' : '#22c55e',
                    type: 'dashed',
                } as const,
                tooltip: { formatter: 'Current Time' }
            });
        }
        
        if (endTime <= xAxisMax) {
             markLineData.push({
                xAxis: endTime,
                lineStyle: {
                    color: isDark ? '#f87171' : '#ef4444',
                    type: 'solid',
                } as const,
                tooltip: { formatter: 'Contest End' }
            });
        }
        
        const markerSeries: SeriesOption = {
            type: 'line',
            data: [],
            silent: true,
            markLine: {
                symbol: 'none',
                label: { show: false },
                data: markLineData,
            },
        };

        return {
            backgroundColor: 'transparent',
            tooltip: {
                trigger: 'axis',
                borderWidth: 0,
                backgroundColor: backgroundColor,
                textStyle: {
                    color: labelColor,
                    fontSize: 12,
                },
                formatter: (params: any) => {
                    const time = format(new Date(params[0].axisValue), 'yyyy-MM-dd HH:mm:ss');
                    let tooltipHtml = `${time}<br/>`;
                    params.sort((a: any, b: any) => (b.value[1] ?? -1) - (a.value[1] ?? -1));
                    params.forEach((param: any) => {
                        if (param.seriesName) {
                            tooltipHtml += `${param.marker} ${param.seriesName}: <strong>${param.value[1]}</strong><br/>`;
                        }
                    });
                    return tooltipHtml;
                }
            },
            legend: {
                data: trendData.map(user => user.nickname),
                textStyle: { color: labelColor },
                bottom: 60,
                type: 'scroll',
            },
            grid: {
                top: 50,
                left: 70,
                right: 50,
                bottom: 110,
            },
            toolbox: {
                show: true,
                showTitle: false,
                feature: {
                    dataZoom: {},
                    restore: {},
                    saveAsImage: {
                        name: 'contest-trend',
                        backgroundColor: isDark ? '#1f2937' : '#fff'
                    },
                },
            },
            xAxis: {
                type: 'time',
                min: startTime,
                max: xAxisMax,
                axisLabel: { color: labelColor },
                splitLine: { show: false },
            },
            yAxis: {
                type: 'value',
                name: 'Score',
                nameTextStyle: { color: labelColor, fontWeight: 'normal' },
                axisLabel: { color: labelColor },
                max: (value) => {
                    const finalMaxScore = Math.max(value.max, absoluteMaxScore);
                    const bufferedMax = finalMaxScore * 1.1;
                    if (bufferedMax <= 100 && bufferedMax > 0) return 100;
                    if (bufferedMax === 0) return 100;
                    return (Math.floor(bufferedMax / 100) + 1) * 100;
                },
                splitLine: { show: true, lineStyle: { color: lineColor, type: 'dashed' } },
            },
            dataZoom: [
                { type: 'inside', xAxisIndex: 0, filterMode: 'none' },
                { type: 'slider', xAxisIndex: 0, bottom: 20, height: 20, showDetail: false },
                { type: 'inside', yAxisIndex: 0, filterMode: 'none' },
                {
                    type: 'slider',
                    yAxisIndex: 0,
                    right: 10,
                    width: 20,
                    showDetail: false,
                },
            ],
            series: [...userSeriesData, markerSeries]
        };
    }, [theme, trendData, contestStartTime, contestEndTime, userSeriesData]);

    return (
        <ReactECharts
            option={chartOptions}
            theme={theme === 'dark' ? 'dark' : 'light'}
            style={{ height: '100%', width: '100%' }}
            notMerge={true}
            lazyUpdate={true}
        />
    );
};

export default EchartsTrendChart;