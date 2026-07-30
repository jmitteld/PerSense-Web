# Fuzzer5 amortization divergence corpus — 800 fresh seeds, 2026-07-30
#
# Sweep: seeds 30000-30799, N=400 each = 320,000 generated, 214,743 compared.
# 50 failure events. Per-case agreement 99.977%. ZERO failures in plain 'pay' mode.
# 74% of failures are in 'noterm' (term solve) — 4.5x enrichment over base rate.
#
# Every line below is a ready-to-run repro:  /tmp/oraclebuild/amort_oracle <args>
# and via the Go side:  M5="<args minus trailing output tokens>" go test -run TestM5Term|TestM5Rows -v

## DISPATCH — Go schedules a screen DOS REFUSES (21 events, 2 root causes)
## Root cause A (18): DOS 'Payment amount is too small to compute number of periods.'
##   = AMORTOP.pas:1343-1344, fancy branch of DetermineLastPaymentDate:
##       RepayFancyLoan(p, usap, h^.loandate, h^.firstdate, nil, false, entire, no_value_calc, 0);
##       if (p > minpmt) then goto ABORT;
##   Go's guard (backward.go ~1728) is 'n > cap || res.FinalPrinc > 1.0' and MISSES because
##   the walk stops AT the wall (n == cap) with the residual folded into the wall payment,
##   so FinalPrinc reads 0.00. Diagnosed on the 492871.04 line: Go emits exactly 96 rows
##   against cap=96 (wall 29/11/2049, quarterly) and totalPaid 758343.02 on a 492871.04 loan.
## Root cause B (3): DOS 'Overflow error: answer too large for this computer's numeric format.'

# DOS: Payment amount is too small to compute number of periods.
amort_oracle 103651.68 0.0991590000 216 12 b365_360 inadv r78 loandmy=29.12.2023 firstdmy=29.1.2024 mor=39 b68=12279.52 b166=24227.30 pre=55:161:12:153.90 targ=52.28 pts=0.017530 payhard=1215.22 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 144585.63 0.1262370000 156 12 b365_360 exact prepaid inadv plusreg r78 loandmy=29.11.2024 firstdmy=29.1.2025 mor=67 b84=35353.28 pre=36:80:26:192.63 skip=5-7 pts=0.009686 payhard=1762.54 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 167915.88 0.0706970000 38 2 b365_360 exact prepaid plusreg r78 loandmy=29.2.2024 firstdmy=29.8.2024 mor=102 b180=50047.36 adj=168:0.0648610000:6232.35 targ=435.09 pts=0.022876 payhard=7407.84 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 187481.51 0.1316620000 120 12 b365_360 exact inadv r78 loandmy=29.12.2024 firstdmy=29.1.2025 mor=59 pre=43:9:12:665.66 skip=1,3,5 pts=0.013873 payhard=2746.73 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 232546.54 0.0737970000 288 12 b365 exact plusreg r78 usa loandmy=30.11.2023 firstdmy=30.1.2024 b221=5233.48 pre=41:277:52:18.27 pre=149:1:12:321.36 targ=204.90 skip=1,7 pts=0.003111 payhard=1495.19 noterm bdump
# DOS: Overflow error: answer too large for this computer's numeric format.
amort_oracle 25931.76 0.1053620000 40 2 r78 loandmy=28.2.2025 firstdmy=28.12.2025 mor=82 b160=5725.31 pre=154:152:26:13.72 pre=130:99:26:22.78 adj=10:0.0321820000:2177.58 adj=178:0.1132270000:2183.88 targ=184.03 pts=0.024705 payhard=1932.52 norate bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 272827.23 0.1294090000 40 2 inadv loandmy=29.2.2024 firstdmy=29.8.2024 b6=34886.46 b18=57093.19 b96=23614.33 pre=48:260:24:178.46 pre=42:90:52:43.56 targ=2054.32 payhard=25265.99 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 277300.99 0.0614980000 36 4 inadv usa loandmy=29.8.2023 firstdmy=29.11.2023 mor=39 b69=67906.89 pre=18:365:52:175.41 pre=3:83:12:699.90 targ=899.89 pts=0.039648 payhard=8994.87 noterm bdump
# DOS: Overflow error: answer too large for this computer's numeric format.
amort_oracle 284182.09 0.0774710000 240 12 prepaid plusreg r78 loandmy=16.8.2025 firstdmy=16.10.2025 mor=15 b88=27452.85 adj=3:0.0264410000:2428.06 adj=51:0.0437040000:3267.42 adj=169:0.0828910000:3104.82 targ=204.42 skip=11-12 pts=0.019511 payhard=2536.49 norate bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 285548.27 0.1189710000 60 4 b365 exact prepaid inadv r78 usa loandmy=29.8.2023 firstdmy=29.11.2023 mor=66 b72=18526.00 b105=32451.60 b141=83320.88 pre=60:43:26:302.68 pre=93:144:24:194.19 targ=955.19 pts=0.023629 payhard=11134.79 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 363847.43 0.0926290000 60 4 b365 exact inadv loandmy=29.8.2025 firstdmy=29.11.2025 mor=60 pre=42:109:12:325.90 pts=0.003290 payhard=9664.78 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 374634.03 0.0895700000 48 2 b365_360 exact prepaid inadv loandmy=29.2.2024 firstdmy=29.8.2024 mor=144 b204=16014.79 b228=72396.49 pre=18:101:52:34.57 targ=4038.77 pts=0.002407 payhard=19865.65 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 420472.21 0.1243680000 100 4 b365_360 prepaid inadv loandmy=29.9.2024 firstdmy=29.11.2024 mor=149 b212=79643.03 b215=120010.21 pre=62:204:52:194.46 pre=134:60:52:32.51 targ=305.02 pts=0.010843 payhard=13648.39 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 425222.22 0.0621940000 48 2 b365_360 exact prepaid inadv r78 usa loandmy=29.2.2024 firstdmy=29.8.2024 mor=114 pre=180:136:52:71.17 targ=527.90 pts=0.038612 payhard=17662.44 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 457719.70 0.0600820000 44 2 loandmy=29.5.2024 firstdmy=29.8.2024 mor=57 b207=26210.82 pre=171:30:26:174.12 pre=117:138:12:444.82 adj=153:0.0201080000:18137.37 targ=882.36 pts=0.013493 payhard=24917.39 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 470283.90 0.0321410000 300 12 b365 exact inadv usa loandmy=30.11.2023 firstdmy=30.1.2024 mor=136 b151=78205.69 pre=112:189:52:42.04 pre=3:247:24:122.55 targ=482.17 skip=11-12 payhard=2964.64 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 475361.58 0.0858020000 32 2 plusreg usa loandmy=30.9.2024 firstdmy=30.8.2025 mor=71 b107=91788.58 pre=11:139:12:227.17 pre=53:29:12:442.25 adj=17:0.0819590000:25011.93 adj=119:0.1159830000:29868.05 targ=6137.23 pts=0.009543 payhard=29691.10 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 51103.99 0.0668030000 276 12 b365 inadv usa loandmy=30.11.2024 firstdmy=30.1.2025 mor=98 b203=5868.63 pre=60:219:52:12.83 targ=30.57 skip=6 pts=0.033742 payhard=333.46 noterm bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 63818.48 0.1379580000 240 12 b365_360 exact prepaid inadv r78 usa loandmy=30.11.2023 firstdmy=30.1.2024 mor=99 b121=8539.18 b166=16965.50 pre=91:139:12:170.82 skip=5-7 pts=0.036562 payhard=796.95 noterm bdump
# DOS: Overflow error: answer too large for this computer's numeric format.
amort_oracle 67482.73 0.0820930000 44 2 exact r78 loandmy=3.2.2024 firstdmy=3.8.2024 b18=13648.60 b162=8254.62 b174=15801.38 pre=24:255:26:28.22 pre=120:90:52:29.53 adj=6:0.0294710000: adj=36:0.0447490000:3679.41 adj=204:0.0953070000:2898.13 pts=0.001375 payhard=3084.22 norate bdump
# DOS: Payment amount is too small to compute number of periods.
amort_oracle 86360.71 0.0773000000 46 2 b365 exact prepaid r78 usa loandmy=30.5.2025 firstdmy=30.8.2025 b111=3442.67 b177=19629.25 b195=10978.67 pre=51:71:12:134.29 targ=599.93 pts=0.002588 payhard=4465.08 noterm bdump

## SOLVETERM — solved term/last-date differs (15 events)
amort_oracle 119546.47 0.0572320000 156 12 exact r78 usa loandmy=19.7.2025 firstdmy=19.9.2025 b35=31701.39 b46=28497.35 b67=11529.14 pre=37:106:24:123.77 adj=74:0.1394630000:833.47 targ=151.87 pts=0.015560 payhard=1463.89 noterm bdump
amort_oracle 122096.18 0.0692920000 92 4 b365_360 prepaid usa loandmy=28.6.2025 firstdmy=28.8.2025 mor=83 b113=26486.63 b206=3599.94 b218=11981.80 pre=173:71:12:179.09 pre=125:91:12:96.27 adj=23::2099.70 adj=104:0.1309480000: adj=128:0.0978690000: targ=645.98 pts=0.003638 payhard=3414.01 non lastdmy=28.5.2048 bdump
amort_oracle 123409.51 0.0312210000 18 2 b365_360 exact usa loandmy=29.2.2024 firstdmy=29.8.2024 mor=36 b60=34808.98 pre=6:93:26:57.07 adj=24:0.1240490000:9296.02 targ=950.45 pts=0.030472 payhard=10684.72 noterm bdump
amort_oracle 131815.81 0.1321370000 14 1 exact prepaid usa loandmy=19.7.2025 firstdmy=19.3.2027 mor=92 b104=32532.68 b116=20526.60 b140=19949.93 pre=80:30:26:49.57 pre=44:26:26:42.13 adj=56:0.0891480000:16497.48 adj=68:0.0919810000:30487.99 pts=0.016566 payhard=26792.65 noterm bdump
amort_oracle 158732.85 0.0928810000 240 12 b365 exact plusreg usa loandmy=16.11.2025 firstdmy=16.12.2025 mor=44 b87=8382.72 b123=23948.46 b175=25692.31 pre=118:32:12:326.70 pre=3:248:26:32.02 adj=34:0.0287280000:1471.80 adj=108:0.1480010000:1493.72 targ=252.71 skip=1,7 payhard=1655.81 noterm bdump
amort_oracle 206672.21 0.0531500000 108 12 b365_360 usa loandmy=30.6.2023 firstdmy=30.8.2023 mor=46 pre=75:25:26:226.79 pre=24:77:24:204.93 adj=3:0.0295450000: adj=11:0.1296340000: targ=236.73 skip=11-12 pts=0.008124 payhard=2231.51 non lastdmy=30.7.2032 bdump
amort_oracle 215717.92 0.0326730000 192 12 b365 exact r78 loandmy=30.11.2025 firstdmy=30.1.2026 mor=73 b93=56451.66 pre=40:78:26:141.82 pre=90:122:26:131.75 adj=54::2009.15 adj=72:0.1106410000: adj=152:0.0930880000: targ=98.71 pts=0.029137 payhard=1458.41 non lastdmy=30.12.2041 bdump
amort_oracle 256861.09 0.0479440000 28 2 b365_360 exact prepaid r78 usa loandmy=2.4.2023 firstdmy=2.4.2024 mor=30 pre=78:81:24:77.18 adj=66:0.0729100000:10824.51 adj=120:0.0699940000:15623.90 pts=0.033918 payhard=16736.55 noterm bdump
amort_oracle 262940.16 0.0660370000 76 4 exact usa loandmy=28.8.2025 firstdmy=28.11.2025 mor=33 b114=30438.91 b144=19872.55 pre=126:52:12:296.16 pre=48:198:52:47.54 adj=18:0.1485520000: adj=165:0.0308870000: targ=728.49 pts=0.014635 payhard=5595.76 non lastdmy=28.8.2044 bdump
amort_oracle 349484.49 0.0312340000 14 1 b365_360 r78 usa loandmy=14.11.2025 firstdmy=14.7.2026 mor=8 b44=10974.36 b56=99693.13 b116=64515.83 pre=92:82:26:183.48 adj=32:0.0908660000:40880.15 adj=80:0.0906580000:25199.58 adj=104:0.1409470000:40114.95 targ=7679.94 pts=0.018291 payhard=37143.21 noterm bdump
amort_oracle 370302.30 0.0944460000 11 1 b365 usa loandmy=12.12.2025 firstdmy=12.7.2027 pre=43:49:12:741.88 adj=55:0.0402030000:64906.28 adj=103:0.1137170000:76116.86 targ=7429.51 pts=0.032172 payhard=68242.01 noterm bdump
amort_oracle 394736.11 0.0309750000 26 2 exact inadv plusreg r78 loandmy=29.5.2024 firstdmy=29.8.2024 mor=45 b81=36005.82 b87=77274.31 b117=50755.09 pre=33:37:12:741.46 pre=57:94:12:594.12 targ=3426.02 pts=0.000392 payhard=24468.37 noterm bdump
amort_oracle 448945.34 0.0766890000 44 4 b365 plusreg r78 loandmy=30.9.2025 firstdmy=30.1.2026 mor=52 b73=64558.72 b76=115829.91 b85=46793.68 adj=43:0.0222530000: adj=61:0.0814680000: adj=70::17028.55 pts=0.014426 payhard=18381.08 non lastdmy=30.10.2036 bdump
amort_oracle 51115.78 0.0507420000 228 12 b365_360 exact plusreg r78 loandmy=29.3.2025 firstdmy=29.5.2025 mor=71 b90=11883.92 pre=7:104:12:21.86 adj=66:0.1272120000: adj=83:0.1287580000: adj=113:0.0582090000: targ=12.93 skip=2,8,11 pts=0.025220 payhard=304.45 non lastdmy=29.4.2044 bdump
amort_oracle 70244.92 0.1176260000 11 1 b365 inadv r78 loandmy=23.4.2025 firstdmy=23.6.2026 pts=0.009929 payhard=12383.04 noterm bdump

## TOTALS — schedule totals differ (13 events)
amort_oracle 115422.66 0.0838700000 92 4 b365_360 exact r78 usa loandmy=29.8.2024 firstdmy=29.11.2024 mor=114 b144=20983.81 pre=108:34:12:165.68 pre=15:395:26:58.82 adj=180:0.0906530000: targ=333.98 pts=0.030965 bdump
amort_oracle 119112.22 0.1199990000 20 1 b365_360 plusreg r78 usa loandmy=21.5.2025 firstdmy=21.5.2026 mor=48 b84=13942.03 b108=14151.93 b120=21024.86 pre=132:237:52:72.47 adj=72:0.1036410000:19902.47 targ=1695.61 payhard=15439.80 noterm bdump
amort_oracle 146883.28 0.0799520000 17 1 exact prepaid inadv plusreg r78 loandmy=16.5.2023 firstdmy=16.11.2024 b126=25924.40 pre=66:52:26:43.79 pre=78:87:12:256.00 targ=1292.15 payhard=17096.46 noterm bdump
amort_oracle 247932.54 0.1026650000 168 12 exact prepaid plusreg r78 loandmy=11.8.2023 firstdmy=11.10.2023 b87=49900.36 b131=59044.19 pre=55:17:12:271.91 pre=25:80:26:141.89 adj=80:0.0740520000:3218.46 targ=516.92 skip=2,8,11 pts=0.024514 payhard=3630.59 noterm bdump
amort_oracle 263916.32 0.1320950000 20 1 b365_360 exact plusreg r78 loandmy=3.4.2025 firstdmy=3.4.2026 b24=5850.36 b180=15490.15 b192=39294.22 pre=156:59:12:691.56 adj=60:0.1140340000:29682.25 targ=3091.71 pts=0.010575 payhard=51049.30 noterm bdump
amort_oracle 278635.40 0.1039870000 80 4 b365_360 usa loandmy=13.7.2025 firstdmy=13.10.2025 mor=48 b144=79325.87 b174=42797.88 b180=16089.64 pre=75:33:24:85.87 pre=132:112:52:152.65 adj=3:0.0493310000:10479.82 targ=1924.95 pts=0.026295 payhard=9375.68 noterm bdump
amort_oracle 302319.64 0.1018280000 19 1 prepaid plusreg r78 usa loandmy=13.3.2024 firstdmy=13.3.2025 b144=65057.19 pre=156:274:52:158.59 adj=48:0.1105360000:28860.85 adj=72:0.0919400000:29350.28 targ=2504.95 pts=0.015315 payhard=43188.69 noterm bdump
amort_oracle 349484.49 0.0312340000 14 1 b365_360 r78 usa loandmy=14.11.2025 firstdmy=14.7.2026 mor=8 b44=10974.36 b56=99693.13 b116=64515.83 pre=92:82:26:183.48 adj=32:0.0908660000:40880.15 adj=80:0.0906580000:25199.58 adj=104:0.1409470000:40114.95 targ=7679.94 pts=0.018291 payhard=37143.21 noterm bdump
amort_oracle 442130.84 0.0598450000 18 2 b365 exact prepaid inadv r78 loandmy=17.7.2024 firstdmy=17.2.2025 mor=55 b67=53269.52 b85=38819.40 pre=13:56:12:161.33 payhard=39005.11 noamt bdump
amort_oracle 451962.13 0.0535630000 120 12 b365 prepaid plusreg loandmy=31.7.2023 firstdmy=31.8.2023 mor=56 b68=71456.77 pre=73:123:52:185.48 pre=11:244:52:212.18 adj=30:0.1256710000: targ=1108.25 skip=2,8,11 pts=0.035998 payhard=5423.48 norate bdump
amort_oracle 465358.89 0.1277780000 16 1 b365 r78 usa loandmy=22.12.2023 firstdmy=22.12.2024 mor=12 b60=97400.90 b120=137917.13 pre=132:57:26:323.99 pre=96:103:24:558.48 adj=108:0.1209600000:83567.59 payhard=91611.37 noterm bdump
amort_oracle 481675.14 0.0507940000 20 2 b365 exact prepaid plusreg loandmy=22.12.2024 firstdmy=22.7.2025 mor=7 b61=67874.88 pre=31:162:24:498.87 adj=19:0.0807940000:38038.75 targ=4727.87 pts=0.016349 payhard=29596.17 noterm bdump
amort_oracle 70244.92 0.1176260000 11 1 b365 inadv r78 loandmy=23.4.2025 firstdmy=23.6.2026 pts=0.009929 payhard=12383.04 noterm bdump

## SOLVEAMOUNT — solved amount differs (1 event)
amort_oracle 442130.84 0.0598450000 18 2 b365 exact prepaid inadv r78 loandmy=17.7.2024 firstdmy=17.2.2025 mor=55 b67=53269.52 b85=38819.40 pre=13:56:12:161.33 payhard=39005.11 noamt bdump
