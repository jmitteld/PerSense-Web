# Widened-axis divergence corpus — 2026-07-30 session C (post prorate fix)
#
# Seeds 51000-51139, N=200, perYrs {1,2,3,4,6,12,24,26,52}.
# 19,429 compared | 18 failure events | 16 distinct cases.
# Per-case agreement 99.9074% (1 in 1079). Same seeds measured 99.3052% before
# today's four fixes.
#
# The residual is NO LONGER frequency-specific: perYr 1,2,3,4,6,12,24,26,52 all
# appear (2,1,1,4,2,1,3,1,1). 12 of 16 are still mode 'noterm'. So the
# sub-monthly defects are closed and what remains is a general term-solve
# residual that was previously masked by them.
#
# Repro:  /tmp/oraclebuild/amort_oracle <args>
#         M5="<args minus bdump>" go test -run TestM5Term -v

amort_oracle 249382.60 0.0989080000 66 6 b365 prepaid loandmy=27.2.2023 firstdmy=27.3.2023 b59=70054.53 b63=67769.12 b83=9711.28 pre=91:13:12:555.18 pre=37:51:24:382.71 adj=87:0.1436960000:6127.82 targ=694.14 pts=0.039350 payhard=7424.78 noterm bdump
amort_oracle 274976.79 0.1104960000 48 4 b365 exact prepaid inadv plusreg r78 loandmy=18.2.2025 firstdmy=18.4.2025 mor=59 b65=62544.88 b98=54041.52 pre=83:17:12:442.08 pre=35:11:52:133.86 pts=0.028308 payhard=13286.23 noterm bdump
amort_oracle 283558.26 0.0571280000 22 2 b365_360 prepaid usa loandmy=18.5.2025 firstdmy=18.10.2025 mor=35 pre=65:56:12:305.57 adj=59:0.0425510000:18929.98 targ=387.00 payhard=23313.39 noterm bdump
amort_oracle 307395.66 0.0361760000 264 24 b365 r78 loandmy=31.5.2025 firstdmy=31.7.2025 targ=50.83 pts=0.003703 payhard=1716.39 norate bdump
amort_oracle 322938.72 0.0860220000 504 24 r78 usa loandmy=31.3.2023 firstdmy=31.5.2023 targ=120.11 payhard=1221.33 norate bdump
amort_oracle 323286.25 0.0526140000 168 12 b365 exact prepaid inadv plusreg loandmy=18.4.2025 firstdmy=18.6.2025 mor=43 b59=71946.72 b100=78847.38 targ=223.65 pts=0.030747 payhard=3090.54 noterm bdump
amort_oracle 34446.68 0.0930540000 10 1 b365_360 exact prepaid r78 usa loandmy=6.1.2023 firstdmy=6.1.2024 b12=9581.86 b36=7260.85 b84=3825.30 pre=72:83:24:43.57 adj=96:0.1048650000:4978.93 targ=307.58 pts=0.001184 payhard=6475.59 noterm bdump
amort_oracle 359053.31 0.0480890000 52 4 b365_360 plusreg usa loandmy=8.1.2024 firstdmy=8.4.2024 mor=75 b90=33765.77 b96=62447.69 pre=45:202:26:292.45 pre=69:2:24:308.16 adj=78:0.0656840000:10948.78 adj=111:0.1362410000:10317.11 targ=1019.66 pts=0.029921 payhard=8084.09 noterm bdump
amort_oracle 361625.51 0.0758280000 456 24 exact loandmy=14.12.2025 firstdmy=14.1.2026 pts=0.011597 payhard=1566.73 non lastdmy=14.12.2063 bdump
amort_oracle 389051.70 0.0530700000 52 4 plusreg loandmy=20.2.2023 firstdmy=20.5.2023 mor=51 b75=65356.71 pre=9:92:12:439.95 adj=72:0.1337300000:14912.25 targ=1592.63 pts=0.015243 payhard=9897.51 noterm bdump
amort_oracle 392280.10 0.1358400000 84 4 plusreg usa loandmy=4.4.2023 firstdmy=4.7.2023 mor=21 b102=60545.47 b183=46817.29 pre=129:48:24:95.69 pre=69:352:26:475.81 adj=171:0.0972600000:18344.92 targ=718.98 pts=0.035045 payhard=13146.91 noterm bdump
amort_oracle 431685.18 0.1171620000 364 26 b365_360 exact loandmy=9.1.2024 firstdmy=9.3.2024 targ=457.25 pts=0.001965 payhard=2913.96 noterm bdump
amort_oracle 436107.86 0.0658950000 520 52 r78 loandmy=30.10.2024 firstdmy=30.11.2024 targ=97.87 pts=0.024939 payhard=1084.54 noterm bdump
amort_oracle 497276.25 0.0779210000 75 3 b365 exact prepaid r78 loandmy=29.6.2023 firstdmy=29.10.2023 mor=4 b24=138829.92 b152=53761.02 b184=105774.81 pre=92:341:26:123.85 adj=56:0.0858500000: adj=172:0.0616310000: targ=3601.44 pts=0.035606 payhard=16976.44 non lastdmy=29.6.2048 bdump
amort_oracle 53840.63 0.0305080000 54 6 usa loandmy=22.1.2023 firstdmy=22.4.2023 mor=3 b35=5741.80 b65=2607.44 b69=4032.56 pre=29:60:12:139.86 pre=41:89:26:63.89 adj=81:0.1454140000:1185.21 targ=53.96 pts=0.020646 payhard=1465.94 noterm bdump
amort_oracle 68561.98 0.1220820000 13 1 exact r78 usa loandmy=22.2.2025 firstdmy=22.2.2026 mor=36 b48=2323.02 b108=17501.18 adj=12:0.0617610000:12430.09 adj=84:0.0923650000:14194.52 targ=2147.28 pts=0.001220 payhard=13405.11 noterm bdump
