#!/usr/bin/env bash
# audit_report.sh — SpeechAuditLog の日次レポートを表示する
# Usage: ./scripts/audit_report.sh [YYYYMMDD]

set -euo pipefail

AUDIT_DIR="${HOME}/.sakura-kodama/audit"
DATE="${1:-$(date +%Y%m%d)}"
FILE="${AUDIT_DIR}/speech_${DATE}.jsonl"

if [ ! -f "$FILE" ]; then
  echo "No audit file found: $FILE"
  exit 1
fi

echo "=== Sakura Audit Report: ${DATE} ==="
echo ""

# 全体サマリー
TOTAL=$(wc -l < "$FILE")
SHOWN=$(jq -c 'select(.type=="speech")' "$FILE" | wc -l)
REJECTED=$(jq -c 'select(.type=="rejected" or .type=="batch_rejected")' "$FILE" | wc -l)

echo "── Overview ──"
printf "  Total records : %d\n" "$TOTAL"
printf "  Shown         : %d\n" "$SHOWN"
printf "  Rejected      : %d\n" "$REJECTED"
if [ "$((SHOWN + REJECTED))" -gt 0 ]; then
  RATE=$(echo "scale=1; $REJECTED * 100 / ($SHOWN + $REJECTED)" | bc)
  printf "  Reject rate   : %s%%  (target: <15%%)\n" "$RATE"
fi
echo ""

# 表示セリフの文字数分布
echo "── Char length (shown speeches) ──"
jq -r 'select(.type=="speech") | .chars' "$FILE" | sort -n | awk '
  BEGIN { n=0; sum=0; min=9999; max=0 }
  {
    n++; sum+=$1;
    if ($1 < min) min=$1;
    if ($1 > max) max=$1;
    vals[n]=$1
  }
  END {
    avg = (n>0) ? sum/n : 0;
    mid = (n%2==1) ? vals[int(n/2)+1] : (vals[n/2]+vals[n/2+1])/2;
    under40 = 0; over60 = 0;
    for (i=1;i<=n;i++) {
      if (vals[i]<=40) under40++;
      if (vals[i]>60) over60++;
    }
    printf "  count=%d  min=%d  max=%d  avg=%.1f  median=%.0f\n", n, min, max, avg, mid;
    printf "  <=40chars: %d (%.0f%%)  >60chars: %d (%.0f%%)\n",
      under40, (n>0)?under40*100/n:0, over60, (n>0)?over60*100/n:0;
  }
'
echo "  (target: median < 40)"
echo ""

# ソース別内訳
echo "── Source breakdown ──"
jq -r 'select(.type=="speech") | .source' "$FILE" \
  | LC_ALL=C sort | LC_ALL=C uniq -c | LC_ALL=C sort -rn \
  | awk '{ printf "  %-10s: %d\n", $2, $1 }'
echo ""

# リジェクトした validator 上位
echo "── Top rejection reasons ──"
jq -r 'select(.type=="rejected" or .type=="batch_rejected") | .rejected_by' "$FILE" \
  | LC_ALL=C sort | LC_ALL=C uniq -c | LC_ALL=C sort -rn | head -10 \
  | awk '{ printf "  %4d  %s\n", $1, $2 }'
echo ""

# reason別の表示数
echo "── Speech count by reason ──"
jq -r 'select(.type=="speech") | .reason' "$FILE" \
  | LC_ALL=C sort | LC_ALL=C uniq -c | LC_ALL=C sort -rn | head -10 \
  | awk '{ printf "  %4d  %s\n", $1, $2 }'
echo ""

# 重複チェック: 同じセリフが何回表示されたか（LC_ALL=C で日本語を正確に比較）
echo "── Duplicate speeches (shown 3+ times) ──"
jq -r 'select(.type=="speech") | .speech // ""' "$FILE" \
  | LC_ALL=C sort | LC_ALL=C uniq -c | LC_ALL=C sort -rn \
  | awk '$1 >= 3 { printf "  %3dx  %s\n", $1, $2 }' | head -10
echo ""

# リリース基準チェック
echo "── Release criteria ──"
REJECT_RATE=$(echo "scale=1; $REJECTED * 100 / ($SHOWN + $REJECTED + 1)" | bc 2>/dev/null || echo "?")
FALLBACK=$(jq -c 'select(.type=="speech" and .source=="fallback")' "$FILE" | wc -l)
jq -r 'select(.type=="speech") | .chars' "$FILE" | sort -n | awk -v shown="$SHOWN" -v fallback="$FALLBACK" -v reject="$REJECTED" '
  BEGIN { n=0 }
  { n++; vals[n]=$1 }
  END {
    mid = (n>0) ? ((n%2==1) ? vals[int(n/2)+1] : (vals[n/2]+vals[n/2+1])/2) : 0;
    total = shown + reject;
    rrate = (total>0) ? reject*100/total : 0;
    frate = (shown>0) ? fallback*100/shown : 0;
    printf "  Reject rate   : %.0f%%  %s\n", rrate,  (rrate<15  ? "✅" : "❌ target:<15%");
    printf "  Median chars  : %.0f    %s\n", mid,    (mid<40    ? "✅" : "❌ target:<40");
    printf "  Fallback rate : %.0f%%  %s\n", frate,  (frate<20  ? "✅" : "❌ target:<20%");
  }
'
echo ""
echo "Log file: $FILE"
