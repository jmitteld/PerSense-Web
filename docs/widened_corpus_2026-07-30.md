# Widened-axis divergence corpus — 2026-07-30 (after the day's three fixes)
#
# Sweep: seeds 51000-51139, N=200, perYrs {1,2,3,4,6,12,24,26,52}.
# 28,000 generated, 19,429 compared, 135 failure events, 116 distinct cases.
# Per-case agreement 99.3052% (1 in 143) — the honest figure once DOS's full
# payment-frequency set is sampled. The 99.977% measured on 2026-07-30 morning
# was on perYrs {1,2,4,12} only, i.e. 4 of DOS's 9 frequencies.
#
# CONCENTRATION: 112 of 116 are mode 'noterm' (term solve); 105 of 116 are at
# perYr 24/26/52. Direction is uniform: Go's solved term is LONGER than DOS's
# by 1-16%.
#
# ROOT CAUSE, characterised but NOT fixed: the term-solve STOP RULE, not the walk.
# Refeeding DOS's own solved term as a fixed term makes the two agree TO THE CENT.
# Worked example (peryr 52):
#   solved:  DOS 7/8/2030 n=349 | Go 8/26/2030 n=356
#   at n=349 fixed: DOS interest 39008.36 paid 146023.13
#                   Go  interest 39008.36 paid 146023.13   <- exact
# So Go's probe walk runs past DOS's stop point. DOS stops on
#   ((not lastok) or (Output<>nil)) and (WhenToStop^.principal = 0)
#   (AMORTOP.pas:1219) — the sub-minpmt fold — whereas Go relies on its own
# engine's early-payoff termination inside a FORCED-term walk. Port DOS's
# condition into solveFancyTermFromPayment's probe.
#
# Each line below is a ready-to-run repro:  /tmp/oraclebuild/amort_oracle <args>
# Go side:  M5="<args minus bdump>" go test -run TestM5Term -v

amort_oracle 103483.45 0.1035840000 650 26 b365_360 exact prepaid inadv plusreg loandmy=30.5.2025 firstdmy=30.7.2025 pts=0.012591 payhard=456.21 noterm bdump
amort_oracle 106777.64 0.1068280000 520 26 inadv plusreg loandmy=21.9.2023 firstdmy=21.10.2023 pts=0.029533 payhard=441.04 noterm bdump
amort_oracle 107014.77 0.0897530000 468 52 b365_360 exact prepaid inadv usa loandmy=6.9.2023 firstdmy=6.11.2023 pts=0.013492 payhard=409.93 noterm bdump
amort_oracle 107632.88 0.1237020000 988 52 inadv r78 usa loandmy=21.11.2025 firstdmy=21.12.2025 pts=0.020423 payhard=273.22 noterm bdump
amort_oracle 116230.27 0.0885900000 504 24 b365_360 prepaid inadv usa loandmy=6.3.2023 firstdmy=6.4.2023 pts=0.011530 payhard=492.49 noterm bdump
amort_oracle 119995.77 0.1243850000 336 24 inadv plusreg usa loandmy=16.6.2023 firstdmy=16.8.2023 pts=0.013308 payhard=700.87 noterm bdump
amort_oracle 120059.30 0.0791900000 528 24 b365 prepaid inadv plusreg loandmy=24.1.2023 firstdmy=24.3.2023 pts=0.023831 payhard=468.60 noterm bdump
amort_oracle 121782.81 0.1244340000 624 26 b365 exact inadv r78 usa loandmy=19.10.2025 firstdmy=19.12.2025 pts=0.018331 payhard=623.63 noterm bdump
amort_oracle 130208.39 0.1040130000 264 24 b365 inadv usa loandmy=2.9.2024 firstdmy=2.10.2024 pts=0.007339 payhard=902.67 noterm bdump
amort_oracle 131062.50 0.1293280000 520 52 b365_360 exact inadv r78 usa loandmy=1.4.2025 firstdmy=1.5.2025 pts=0.015393 payhard=446.45 noterm bdump
amort_oracle 131671.09 0.1219190000 364 26 b365 prepaid inadv plusreg r78 loandmy=24.7.2023 firstdmy=24.8.2023 pts=0.010148 payhard=1008.32 noterm bdump
amort_oracle 138640.18 0.1150100000 676 52 prepaid inadv plusreg r78 loandmy=14.3.2025 firstdmy=14.5.2025 pts=0.010211 payhard=501.32 noterm bdump
amort_oracle 139871.98 0.0762730000 468 26 exact prepaid inadv loandmy=9.8.2025 firstdmy=9.9.2025 pts=0.014246 payhard=491.23 noterm bdump
amort_oracle 140950.09 0.1029250000 504 24 b365_360 exact inadv plusreg r78 loandmy=22.3.2025 firstdmy=22.4.2025 pts=0.000890 payhard=693.50 noterm bdump
amort_oracle 150560.58 0.1346650000 1248 52 inadv plusreg usa loandmy=9.9.2023 firstdmy=9.10.2023 pts=0.023032 payhard=539.23 noterm bdump
amort_oracle 152047.02 0.0497920000 384 24 b365 prepaid inadv loandmy=12.3.2025 firstdmy=12.4.2025 pts=0.002406 payhard=519.84 noterm bdump
amort_oracle 155737.68 0.1048400000 468 52 exact inadv r78 usa loandmy=21.11.2024 firstdmy=21.12.2024 pts=0.031024 payhard=480.49 noterm bdump
amort_oracle 156686.49 0.0800650000 456 24 b365_360 prepaid inadv plusreg usa loandmy=10.9.2025 firstdmy=10.11.2025 pts=0.035604 payhard=631.43 noterm bdump
amort_oracle 157907.45 0.1180300000 528 24 b365_360 exact inadv loandmy=12.12.2025 firstdmy=12.1.2026 pts=0.012724 payhard=1070.17 noterm bdump
amort_oracle 162824.46 0.1333750000 338 26 b365_360 prepaid inadv plusreg r78 loandmy=6.3.2024 firstdmy=6.4.2024 pts=0.027569 payhard=1286.78 noterm bdump
amort_oracle 171504.56 0.1041020000 1092 52 b365 exact prepaid inadv r78 usa loandmy=20.10.2025 firstdmy=20.12.2025 pts=0.032778 payhard=518.05 noterm bdump
amort_oracle 175693.98 0.0464220000 288 24 prepaid inadv r78 usa loandmy=11.2.2024 firstdmy=11.4.2024 pts=0.002942 payhard=915.12 noterm bdump
amort_oracle 177483.21 0.0832420000 528 24 b365_360 exact inadv usa loandmy=26.9.2024 firstdmy=26.11.2024 pts=0.016791 payhard=655.90 noterm bdump
amort_oracle 185132.78 0.0777010000 600 24 exact prepaid inadv loandmy=14.12.2024 firstdmy=14.1.2025 pts=0.017472 payhard=681.08 noterm bdump
amort_oracle 196854.26 0.0538520000 264 24 b365_360 exact prepaid inadv r78 loandmy=20.1.2023 firstdmy=20.3.2023 pts=0.025282 payhard=864.70 noterm bdump
amort_oracle 198691.04 0.1143840000 456 24 inadv usa loandmy=5.4.2025 firstdmy=5.6.2025 pts=0.038540 payhard=1053.95 noterm bdump
amort_oracle 204327.41 0.0334320000 468 52 b365_360 exact prepaid inadv plusreg r78 loandmy=11.1.2024 firstdmy=11.3.2024 pts=0.035625 payhard=603.06 noterm bdump
amort_oracle 206207.45 0.0346560000 494 26 inadv usa loandmy=3.11.2024 firstdmy=3.1.2025 pts=0.014728 payhard=667.01 noterm bdump
amort_oracle 208862.94 0.0728270000 312 24 exact inadv plusreg r78 usa loandmy=3.12.2025 firstdmy=3.2.2026 pts=0.037391 payhard=977.32 noterm bdump
amort_oracle 208866.81 0.0630050000 832 52 prepaid inadv r78 usa loandmy=23.6.2024 firstdmy=23.7.2024 pts=0.039236 payhard=477.33 noterm bdump
amort_oracle 230292.52 0.0964960000 234 26 inadv plusreg loandmy=16.3.2025 firstdmy=16.5.2025 pts=0.017528 payhard=1588.63 noterm bdump
amort_oracle 232325.43 0.1091100000 312 24 b365 exact prepaid inadv plusreg usa loandmy=29.8.2024 firstdmy=29.9.2024 pts=0.014449 payhard=1647.47 noterm bdump
amort_oracle 232543.70 0.0903850000 360 24 b365_360 exact inadv plusreg loandmy=29.10.2023 firstdmy=29.12.2023 pts=0.005534 payhard=1186.39 noterm bdump
amort_oracle 245052.44 0.1193210000 504 24 b365_360 exact prepaid inadv plusreg usa loandmy=4.10.2025 firstdmy=4.12.2025 pts=0.039951 payhard=1524.37 noterm bdump
amort_oracle 246925.92 0.1258840000 832 52 b365_360 exact prepaid inadv plusreg r78 loandmy=30.4.2025 firstdmy=30.6.2025 pts=0.007237 payhard=634.68 noterm bdump
amort_oracle 247069.69 0.0482730000 390 26 b365_360 exact inadv loandmy=30.12.2025 firstdmy=28.2.2026 pts=0.009115 payhard=1095.03 noterm bdump
amort_oracle 249382.60 0.0989080000 66 6 b365 prepaid loandmy=27.2.2023 firstdmy=27.3.2023 b59=70054.53 b63=67769.12 b83=9711.28 pre=91:13:12:555.18 pre=37:51:24:382.71 adj=87:0.1436960000:6127.82 targ=694.14 pts=0.039350 payhard=7424.78 noterm bdump
amort_oracle 251192.63 0.1295480000 312 26 b365 inadv plusreg usa loandmy=8.8.2025 firstdmy=8.9.2025 pts=0.021796 payhard=2022.81 noterm bdump
amort_oracle 254067.14 0.0627730000 456 24 b365 exact prepaid inadv r78 usa loandmy=12.10.2023 firstdmy=12.11.2023 pts=0.038484 payhard=1005.21 noterm bdump
amort_oracle 254258.26 0.0591180000 216 24 b365_360 inadv plusreg r78 loandmy=14.11.2025 firstdmy=14.12.2025 pts=0.004865 payhard=1860.56 noterm bdump
amort_oracle 255664.10 0.0745050000 884 52 inadv plusreg usa loandmy=27.1.2023 firstdmy=27.2.2023 pts=0.037823 payhard=462.35 noterm bdump
amort_oracle 25804.81 0.1100600000 576 24 prepaid inadv plusreg r78 loandmy=19.9.2023 firstdmy=19.11.2023 pts=0.035102 payhard=166.92 noterm bdump
amort_oracle 261639.86 0.1349740000 624 52 prepaid inadv plusreg loandmy=6.1.2023 firstdmy=6.2.2023 pts=0.001113 payhard=985.33 noterm bdump
amort_oracle 262347.84 0.1237690000 504 24 b365 inadv plusreg r78 usa loandmy=24.10.2024 firstdmy=24.12.2024 pts=0.036658 payhard=1420.85 noterm bdump
amort_oracle 266667.51 0.1047180000 192 24 exact inadv r78 loandmy=28.12.2025 firstdmy=28.2.2026 pts=0.039422 payhard=2289.28 noterm bdump
amort_oracle 27036.01 0.0416810000 988 52 b365_360 prepaid inadv plusreg usa loandmy=22.10.2023 firstdmy=22.11.2023 pts=0.036340 payhard=43.72 noterm bdump
amort_oracle 272284.14 0.1312870000 520 52 b365_360 exact prepaid inadv r78 loandmy=10.9.2025 firstdmy=10.11.2025 pts=0.013977 payhard=1128.97 noterm bdump
amort_oracle 274976.79 0.1104960000 48 4 b365 exact prepaid inadv plusreg r78 loandmy=18.2.2025 firstdmy=18.4.2025 mor=59 b65=62544.88 b98=54041.52 pre=83:17:12:442.08 pre=35:11:52:133.86 pts=0.028308 payhard=13286.23 noterm bdump
amort_oracle 279807.95 0.0595850000 546 26 b365_360 inadv usa loandmy=19.1.2025 firstdmy=19.3.2025 pts=0.035654 payhard=949.42 noterm bdump
amort_oracle 283558.26 0.0571280000 22 2 b365_360 prepaid usa loandmy=18.5.2025 firstdmy=18.10.2025 mor=35 pre=65:56:12:305.57 adj=59:0.0425510000:18929.98 targ=387.00 payhard=23313.39 noterm bdump
amort_oracle 292246.92 0.0727570000 1196 52 b365 exact prepaid inadv r78 usa loandmy=24.8.2024 firstdmy=24.10.2024 pts=0.031433 payhard=455.67 noterm bdump
amort_oracle 294842.51 0.0798980000 390 26 b365 exact inadv plusreg loandmy=20.2.2025 firstdmy=20.4.2025 pts=0.022826 payhard=1514.67 noterm bdump
amort_oracle 300336.95 0.0756440000 456 24 exact inadv plusreg r78 usa loandmy=24.5.2023 firstdmy=24.6.2023 pts=0.027163 payhard=1249.40 noterm bdump
amort_oracle 30333.62 0.0677010000 264 24 exact inadv plusreg usa loandmy=19.11.2025 firstdmy=19.12.2025 pts=0.029492 payhard=200.49 noterm bdump
amort_oracle 307395.66 0.0361760000 264 24 b365 r78 loandmy=31.5.2025 firstdmy=31.7.2025 targ=50.83 pts=0.003703 payhard=1716.39 norate bdump
amort_oracle 308089.60 0.1002250000 288 24 b365_360 inadv r78 loandmy=17.9.2024 firstdmy=17.10.2024 pts=0.035607 payhard=1673.29 noterm bdump
amort_oracle 321319.50 0.1350230000 408 24 b365_360 exact prepaid inadv plusreg usa loandmy=18.2.2024 firstdmy=18.4.2024 pts=0.025968 payhard=1844.90 noterm bdump
amort_oracle 322938.72 0.0860220000 504 24 r78 usa loandmy=31.3.2023 firstdmy=31.5.2023 targ=120.11 payhard=1221.33 norate bdump
amort_oracle 323286.25 0.0526140000 168 12 b365 exact prepaid inadv plusreg loandmy=18.4.2025 firstdmy=18.6.2025 mor=43 b59=71946.72 b100=78847.38 targ=223.65 pts=0.030747 payhard=3090.54 noterm bdump
amort_oracle 324600.88 0.1030030000 364 26 prepaid inadv r78 usa loandmy=7.2.2025 firstdmy=7.4.2025 pts=0.014044 payhard=1961.20 noterm bdump
amort_oracle 328429.17 0.0664940000 552 24 b365_360 exact inadv plusreg r78 usa loandmy=23.10.2024 firstdmy=23.11.2024 pts=0.018296 payhard=1105.23 noterm bdump
amort_oracle 328546.90 0.0841080000 598 26 b365 inadv usa loandmy=14.8.2023 firstdmy=14.10.2023 pts=0.021959 payhard=1228.00 noterm bdump
amort_oracle 329778.79 0.0352150000 208 26 b365 exact prepaid inadv plusreg loandmy=26.8.2023 firstdmy=26.10.2023 pts=0.018322 payhard=2252.89 noterm bdump
amort_oracle 338491.79 0.0868180000 546 26 inadv plusreg loandmy=8.6.2023 firstdmy=8.7.2023 pts=0.021375 payhard=1787.77 noterm bdump
amort_oracle 34446.68 0.0930540000 10 1 b365_360 exact prepaid r78 usa loandmy=6.1.2023 firstdmy=6.1.2024 b12=9581.86 b36=7260.85 b84=3825.30 pre=72:83:24:43.57 adj=96:0.1048650000:4978.93 targ=307.58 pts=0.001184 payhard=6475.59 noterm bdump
amort_oracle 35829.75 0.0700560000 728 52 exact prepaid inadv usa loandmy=17.3.2023 firstdmy=17.5.2023 pts=0.019162 payhard=88.31 noterm bdump
amort_oracle 359053.31 0.0480890000 52 4 b365_360 plusreg usa loandmy=8.1.2024 firstdmy=8.4.2024 mor=75 b90=33765.77 b96=62447.69 pre=45:202:26:292.45 pre=69:2:24:308.16 adj=78:0.0656840000:10948.78 adj=111:0.1362410000:10317.11 targ=1019.66 pts=0.029921 payhard=8084.09 noterm bdump
amort_oracle 359741.60 0.0311950000 1144 52 b365_360 inadv usa loandmy=25.6.2024 firstdmy=25.7.2024 pts=0.034713 payhard=551.94 noterm bdump
amort_oracle 360008.96 0.1107320000 1144 52 b365_360 inadv r78 usa loandmy=19.3.2023 firstdmy=19.4.2023 pts=0.018757 payhard=1001.00 noterm bdump
amort_oracle 361625.51 0.0758280000 456 24 exact loandmy=14.12.2025 firstdmy=14.1.2026 pts=0.011597 payhard=1566.73 non lastdmy=14.12.2063 bdump
amort_oracle 364630.72 0.1246510000 208 26 b365_360 inadv plusreg usa loandmy=14.9.2023 firstdmy=14.11.2023 pts=0.017915 payhard=3277.33 noterm bdump
amort_oracle 364674.89 0.0313870000 780 52 b365_360 exact prepaid inadv r78 loandmy=19.7.2025 firstdmy=19.9.2025 pts=0.023134 payhard=520.92 noterm bdump
amort_oracle 366793.86 0.0403040000 1248 52 b365 exact prepaid inadv plusreg loandmy=14.4.2023 firstdmy=14.6.2023 pts=0.008180 payhard=469.92 noterm bdump
amort_oracle 373370.21 0.1126160000 360 24 b365_360 exact prepaid inadv plusreg usa loandmy=5.11.2023 firstdmy=5.1.2024 pts=0.018845 payhard=1861.28 noterm bdump
amort_oracle 373606.85 0.0686780000 216 24 prepaid inadv plusreg r78 loandmy=2.8.2024 firstdmy=2.9.2024 pts=0.014911 payhard=2599.06 noterm bdump
amort_oracle 384536.40 0.1125930000 528 24 b365 prepaid inadv r78 usa loandmy=6.1.2023 firstdmy=6.3.2023 pts=0.029678 payhard=2461.60 noterm bdump
amort_oracle 387277.92 0.0513740000 552 24 b365_360 prepaid inadv plusreg r78 loandmy=3.7.2025 firstdmy=3.9.2025 pts=0.007839 payhard=1480.02 noterm bdump
amort_oracle 388485.92 0.1388460000 600 24 b365 exact prepaid inadv plusreg r78 loandmy=28.3.2024 firstdmy=28.4.2024 pts=0.019947 payhard=2613.00 noterm bdump
amort_oracle 389051.70 0.0530700000 52 4 plusreg loandmy=20.2.2023 firstdmy=20.5.2023 mor=51 b75=65356.71 pre=9:92:12:439.95 adj=72:0.1337300000:14912.25 targ=1592.63 pts=0.015243 payhard=9897.51 noterm bdump
amort_oracle 392280.10 0.1358400000 84 4 plusreg usa loandmy=4.4.2023 firstdmy=4.7.2023 mor=21 b102=60545.47 b183=46817.29 pre=129:48:24:95.69 pre=69:352:26:475.81 adj=171:0.0972600000:18344.92 targ=718.98 pts=0.035045 payhard=13146.91 noterm bdump
amort_oracle 404053.25 0.0844840000 600 24 b365 prepaid inadv plusreg r78 usa loandmy=1.2.2025 firstdmy=1.4.2025 pts=0.017174 payhard=2041.26 noterm bdump
amort_oracle 405715.94 0.0359720000 468 26 exact inadv plusreg r78 loandmy=17.7.2024 firstdmy=17.8.2024 pts=0.027642 payhard=1200.32 noterm bdump
amort_oracle 407562.03 0.0450550000 624 52 b365 exact inadv plusreg r78 usa loandmy=21.1.2025 firstdmy=21.2.2025 pts=0.020505 payhard=976.78 noterm bdump
amort_oracle 411030.99 0.0779150000 624 52 b365 prepaid inadv usa loandmy=30.4.2025 firstdmy=30.5.2025 pts=0.034032 payhard=1282.71 noterm bdump
amort_oracle 411331.84 0.0633490000 546 26 prepaid inadv plusreg r78 loandmy=11.4.2023 firstdmy=11.6.2023 pts=0.032515 payhard=1543.98 noterm bdump
amort_oracle 412965.38 0.1362760000 624 26 exact inadv r78 loandmy=13.8.2024 firstdmy=13.10.2024 pts=0.000069 payhard=2753.53 noterm bdump
amort_oracle 417935.77 0.0426550000 234 26 b365_360 inadv plusreg loandmy=5.11.2023 firstdmy=5.1.2024 pts=0.028821 payhard=2532.46 noterm bdump
amort_oracle 420771.70 0.0756290000 650 26 b365 exact prepaid inadv plusreg r78 usa loandmy=30.5.2023 firstdmy=30.7.2023 pts=0.034786 payhard=1358.62 noterm bdump
amort_oracle 423568.04 0.0728690000 1248 52 b365_360 prepaid inadv plusreg loandmy=9.6.2025 firstdmy=9.7.2025 pts=0.012233 payhard=875.59 noterm bdump
amort_oracle 427418.49 0.0450230000 1248 52 exact prepaid inadv usa loandmy=6.9.2023 firstdmy=6.11.2023 pts=0.010987 payhard=556.11 noterm bdump
amort_oracle 429608.87 0.0575940000 1092 52 b365 exact inadv plusreg usa loandmy=13.9.2023 firstdmy=13.10.2023 pts=0.025322 payhard=896.97 noterm bdump
amort_oracle 431685.18 0.1171620000 364 26 b365_360 exact loandmy=9.1.2024 firstdmy=9.3.2024 targ=457.25 pts=0.001965 payhard=2913.96 noterm bdump
amort_oracle 43348.33 0.1134840000 1196 52 b365 exact prepaid inadv plusreg r78 loandmy=13.7.2025 firstdmy=13.8.2025 pts=0.005268 payhard=136.89 noterm bdump
amort_oracle 436107.86 0.0658950000 520 52 r78 loandmy=30.10.2024 firstdmy=30.11.2024 targ=97.87 pts=0.024939 payhard=1084.54 noterm bdump
amort_oracle 444052.86 0.0369290000 1248 52 prepaid inadv usa loandmy=11.1.2023 firstdmy=11.2.2023 pts=0.000314 payhard=625.01 noterm bdump
amort_oracle 445109.41 0.1205150000 1196 52 b365_360 exact inadv plusreg loandmy=21.4.2023 firstdmy=21.5.2023 pts=0.021437 payhard=1314.46 noterm bdump
amort_oracle 45052.18 0.0340100000 546 26 b365_360 exact prepaid inadv plusreg loandmy=1.3.2023 firstdmy=1.5.2023 pts=0.025305 payhard=98.38 noterm bdump
amort_oracle 45968.99 0.1136180000 192 24 b365 prepaid inadv usa loandmy=6.11.2025 firstdmy=6.12.2025 pts=0.027740 payhard=442.07 noterm bdump
amort_oracle 463660.97 0.1081090000 624 26 exact inadv loandmy=17.3.2025 firstdmy=17.5.2025 pts=0.038683 payhard=2759.31 noterm bdump
amort_oracle 465939.87 0.1190290000 480 24 exact prepaid inadv r78 usa loandmy=10.7.2023 firstdmy=10.9.2023 pts=0.037052 payhard=3169.94 noterm bdump
amort_oracle 467976.71 0.0711960000 456 24 exact prepaid inadv r78 loandmy=13.12.2025 firstdmy=13.1.2026 pts=0.034818 payhard=2448.97 noterm bdump
amort_oracle 469767.87 0.0807010000 336 24 b365_360 prepaid inadv r78 loandmy=11.7.2023 firstdmy=11.9.2023 pts=0.011606 payhard=2340.49 noterm bdump
amort_oracle 470670.91 0.0566990000 598 26 inadv plusreg r78 loandmy=16.2.2023 firstdmy=16.3.2023 pts=0.030568 payhard=1630.04 noterm bdump
amort_oracle 473585.48 0.0584670000 208 26 b365_360 prepaid inadv plusreg r78 loandmy=29.3.2025 firstdmy=29.4.2025 pts=0.028170 payhard=3819.11 noterm bdump
amort_oracle 483857.28 0.0584790000 576 24 exact inadv usa loandmy=11.4.2023 firstdmy=11.5.2023 pts=0.003499 payhard=1821.79 noterm bdump
amort_oracle 497276.25 0.0779210000 75 3 b365 exact prepaid r78 loandmy=29.6.2023 firstdmy=29.10.2023 mor=4 b24=138829.92 b152=53761.02 b184=105774.81 pre=92:341:26:123.85 adj=56:0.0858500000: adj=172:0.0616310000: targ=3601.44 pts=0.035606 payhard=16976.44 non lastdmy=29.6.2048 bdump
amort_oracle 53840.63 0.0305080000 54 6 usa loandmy=22.1.2023 firstdmy=22.4.2023 mor=3 b35=5741.80 b65=2607.44 b69=4032.56 pre=29:60:12:139.86 pre=41:89:26:63.89 adj=81:0.1454140000:1185.21 targ=53.96 pts=0.020646 payhard=1465.94 noterm bdump
amort_oracle 56348.04 0.1306390000 338 26 exact prepaid inadv plusreg r78 loandmy=17.6.2025 firstdmy=17.8.2025 pts=0.017905 payhard=300.39 noterm bdump
amort_oracle 56640.19 0.0687920000 988 52 exact prepaid inadv r78 loandmy=21.4.2024 firstdmy=21.6.2024 pts=0.020108 payhard=109.40 noterm bdump
amort_oracle 59361.83 0.0664260000 468 52 inadv plusreg usa loandmy=22.4.2025 firstdmy=22.5.2025 pts=0.016501 payhard=178.47 noterm bdump
amort_oracle 68561.98 0.1220820000 13 1 exact r78 usa loandmy=22.2.2025 firstdmy=22.2.2026 mor=36 b48=2323.02 b108=17501.18 adj=12:0.0617610000:12430.09 adj=84:0.0923650000:14194.52 targ=2147.28 pts=0.001220 payhard=13405.11 noterm bdump
amort_oracle 83150.71 0.0613470000 312 24 b365_360 exact prepaid inadv r78 loandmy=25.8.2024 firstdmy=25.10.2024 pts=0.030895 payhard=417.73 noterm bdump
amort_oracle 87960.88 0.1025260000 336 24 b365_360 inadv plusreg r78 loandmy=20.9.2023 firstdmy=20.10.2023 pts=0.035477 payhard=530.38 noterm bdump
amort_oracle 89117.56 0.1392100000 432 24 b365 prepaid inadv loandmy=7.9.2023 firstdmy=7.11.2023 pts=0.023647 payhard=537.84 noterm bdump
amort_oracle 96422.80 0.1298060000 468 52 b365 exact prepaid inadv plusreg loandmy=18.10.2024 firstdmy=18.12.2024 pts=0.035100 payhard=389.04 noterm bdump
amort_oracle 98078.23 0.0618060000 728 52 b365_360 inadv usa loandmy=7.5.2023 firstdmy=7.7.2023 pts=0.018122 payhard=205.75 noterm bdump
