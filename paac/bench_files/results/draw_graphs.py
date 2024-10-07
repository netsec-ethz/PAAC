import re
import pandas as pd
import statistics
import matplotlib.pyplot as plt
import numpy as np


# This file can be used to generate some latex tables and plots from the data
# It includes redundant or mislabelled parts, is not polished, documented or particularly usable,
# and is not intended to be. It is included for personal utility and
# in case someone else may find it useful.
# Use at own risk
# The idea is to first run benchmarks with runner.sh, then run
# this script to turn the benchmark outputs into something more useful

# File names used in the script may need to be adjusted (e.g. Remove the "Final")
# suffix for freshly generated data


def round_mean(v):
    if abs(v) >= 100:
        return round(v)
    else:
        return round(v, 1)


def draw_graph(
    fname,
    y_label,
    category_labels,
    bench_labels,
    values,
    title="",
    lower_vals=None,
    add_height=True,
    yscale="log",
    main_color="cornflowerblue",
    second_color="orangered",
):
    fig, ax = plt.subplots()

    bar_width = 0.25
    fill_ratio = 0.9
    fill_margin = (1 - fill_ratio) / 2
    filled_width = fill_ratio * bar_width
    index = np.arange(len(category_labels))
    for i, category in enumerate(bench_labels):
        bars = ax.bar(
            index * (len(bench_labels) + 1) * bar_width
            + i * bar_width
            + fill_margin * bar_width,
            values[:, i],
            filled_width,
            color=[main_color if v <= 0 else second_color for v in values[:, i]],
        )
        if add_height:
            for bar in bars:
                yval = bar.get_height()
                ax.text(
                    bar.get_x() + bar.get_width(),
                    yval,
                    yval,
                    ha="right",
                    va="bottom",
                    fontsize=8,
                )

    if lower_vals is not None:
        for i, category in enumerate(bench_labels):
            ax.bar(
                index * (len(bench_labels) + 1) * bar_width
                + i * bar_width
                + fill_margin * bar_width,
                lower_vals[:, i],
                filled_width,
                color="orange",
            )

    ax.set_yscale(yscale)
    ax.set_ylabel(y_label)
    if title != "":
        ax.set_title(title)
    xtick_positions = []
    xtick_labels = []
    for i, benchmark in enumerate(category_labels):
        group_pos = index[i] * (len(bench_labels) + 1) * bar_width
        for j in range(len(bench_labels)):
            xtick_positions.append(group_pos + j * bar_width)
            xtick_labels.append(f"{bench_labels[j]}")
        ax.text(
            group_pos + bar_width * (len(bench_labels) - 1) / 2,
            -0.12,
            benchmark,
            ha="center",
            va="top",
            transform=ax.get_xaxis_transform(),
            fontsize=10,
        )

    ax.set_xticks(xtick_positions)
    ax.set_xticklabels(xtick_labels, rotation=45, ha="right", fontsize=7)
    plt.axhline(y=0, color="black", linewidth=0.8)

    plt.savefig("graphs/" + fname, dpi=300, bbox_inches="tight")

    plt.close()


def remove_suffix(text):
    return re.sub(r"(#\d*-?\d*|-?\d+)$", "", text)


def get_paac_nonet_latency_data_dict(fname):
    data_dict = {}
    with open(fname, "r") as file:
        for line in file:
            if not line.startswith("Benchmark"):
                continue
            if "8Routines" not in line:
                continue
            line = line.replace("8Routines", "")
            split_line = line.strip().split()
            bench_name = remove_suffix(split_line[0]).split("/")[1]
            if bench_name not in data_dict:
                data_dict[bench_name] = []
            data_dict[bench_name].append(int(split_line[2]) / 1000)
    for k, v in data_dict.items():
        data_dict[k] = {
            "mean": statistics.mean(v),
            "std": statistics.stdev(v),
        }
    return data_dict


def get_paac_nonet_throughput_data_dict(fname):
    data_dict = {}
    with open(fname, "r") as file:
        for line in file:
            if not line.startswith("Benchmark"):
                continue
            if "8Routines" not in line:
                continue
            line = line.replace("8Routines", "")
            split_line = line.strip().split()
            bench_name = remove_suffix(split_line[0]).split("/")[1]
            if bench_name not in data_dict:
                data_dict[bench_name] = []
            data_dict[bench_name].append(1_000_000.0 / (int(split_line[2]) / 1000))
    for k, v in data_dict.items():
        data_dict[k] = {
            "mean": statistics.mean(v),
            "std": statistics.stdev(v),
        }
    return data_dict


def get_throughput_data_dict(fname):
    data_dict = {}
    with open(fname, "r") as file:
        for line in file:
            if not line.startswith("Benchmark"):
                continue
            line = line.replace("8cores", "")
            split_line = line.strip().split()
            bench_name = remove_suffix(split_line[0]).split("/")[1]
            if bench_name not in data_dict:
                data_dict[bench_name] = []
            data_dict[bench_name].append(1_000_000.0 / (int(split_line[2]) / 1000))
    for k, v in data_dict.items():
        data_dict[k] = {
            "mean": statistics.mean(v),
            "std": statistics.stdev(v),
        }
    return data_dict


def get_data_dict(fname):
    data_dict = {}
    with open(fname, "r") as file:
        for line in file:
            if not line.startswith("Benchmark"):
                continue
            split_line = line.strip().split()
            bench_name = remove_suffix(split_line[0]).split("/")[1]
            if bench_name not in data_dict:
                data_dict[bench_name] = []
            data_dict[bench_name].append(int(split_line[2]) / 1000)
    for k, v in data_dict.items():
        data_dict[k] = {
            "mean": statistics.mean(v),
            "std": statistics.stdev(v),
        }
    return data_dict


def get_no_cache_data_dict(fname, nBuilders, latency):
    data_dict = {}
    with open(fname, "r") as file:
        for line in file:
            if not line.startswith("Benchmark"):
                continue
            if nBuilders not in line:
                continue
            split_line = line.strip().split()
            bench_name = remove_suffix(split_line[0]).split("/")[1]
            if bench_name not in data_dict:
                data_dict[bench_name] = []
            if latency:
                data_dict[bench_name].append(int(split_line[2]) / 1000)
            else:
                data_dict[bench_name].append(1_000_000.0 / (int(split_line[2]) / 1000))

    for k, v in data_dict.items():
        data_dict[k] = {
            "mean": statistics.mean(v),
            "std": statistics.stdev(v),
        }
    return data_dict


def get_multicore_latency_data_dict(fname, num_cores):
    data_dict = {}
    with open(fname, "r") as file:
        for line in file:
            if not line.startswith("Benchmark"):
                continue
            line = line.replace("8cores", "")
            split_line = line.strip().split()
            bench_name = remove_suffix(split_line[0]).split("/")[1]
            if bench_name not in data_dict:
                data_dict[bench_name] = []
            data_dict[bench_name].append(int(split_line[2]) * num_cores / 1000)
    for k, v in data_dict.items():
        data_dict[k] = {
            "mean": statistics.mean(v),
            "std": statistics.stdev(v),
        }
    return data_dict


def get_values_dict(data_dict, bname, latency=False):
    mean_dict = dict()
    std_dict = dict()
    for k, v in data_dict.items():
        if latency != ("Latency" in k):
            continue
        s = k.removeprefix(bname)
        s = s.split("Rules")
        n_rules = int(s[0])
        n_Attrs = int(s[1].split("Attrs")[0])
        if n_rules not in mean_dict:
            mean_dict[n_rules] = dict()
            std_dict[n_rules] = dict()

        mean_dict[n_rules][n_Attrs] = round_mean(v["mean"])
        std_dict[n_rules][n_Attrs] = round(v["std"], 1)

    return mean_dict, std_dict


def get_single_path_values_dict(data_dict, bname, latency=False):
    mean_dict = dict()
    std_dict = dict()
    for k, v in data_dict.items():
        if latency != ("Latency" in k):
            continue
        s = k.removeprefix(bname)
        s = s.split("Rules")
        n_Attrs = int(s[0].split("Hops")[0])
        n_rules = int(s[1].split("Attrs")[0])
        if n_rules not in mean_dict:
            mean_dict[n_rules] = dict()
            std_dict[n_rules] = dict()

        mean_dict[n_rules][n_Attrs] = round(v["mean"], 1)
        std_dict[n_rules][n_Attrs] = round(v["std"], 1)

    return mean_dict, std_dict


def get_values(mean_dict):
    category_labels = sorted(mean_dict)
    bench_labels = sorted(mean_dict[category_labels[0]])

    values = np.zeros((len(category_labels), len(bench_labels)), dtype=float)
    for j, att in enumerate(bench_labels):
        for i, rules in enumerate(category_labels):
            mu = mean_dict[rules][att]
            values[i][j] = mu

    category_labels = [str(cl) + " Rules" for cl in category_labels]
    bench_labels = [str(bl) + " Attr." for bl in bench_labels]
    return category_labels, bench_labels, values


def print_tex_table(mean_dict, std_dict):
    category_labels = sorted(mean_dict)
    bench_labels = sorted(mean_dict[category_labels[0]])

    for rules in category_labels:
        print(f"& \\multicolumn{{2}}{{c|[1.5pt]}}{{{rules}}}", end="")

    print("\\\\\\tabucline[1.5pt]{-}")
    for j, att in enumerate(bench_labels):
        print(att, end="")
        for rules in category_labels:

            print("&", mean_dict[rules][att], end="")
            print("&", f"({std_dict[rules][att]})", end="")
        if j < len(bench_labels) - 1:
            print("\\\\\\hline")

    print("\\\\\\tabucline[1.5pt]{-}")
    print("\\multicolumn{1}{c}{}", end="")
    for rules in category_labels:
        print("& \\multicolumn{1}{r}{$\\mu$}& \\multicolumn{1}{l}{$(\\sigma)$}", end="")
    print()

    category_labels = [str(cl) + " Rules" for cl in category_labels]
    bench_labels = [str(bl) + " Attr." for bl in bench_labels]


def draw_point_graph(
    fname,
    vals,
    r_strs,
    a_strs,
    x_axis,
    y_axis,
    title="",
    x_scale="linear",
    y_scale="linear",
):
    rules = [int(x.split()[0]) for x in r_strs]
    attrs = [int(x.split()[0]) for x in a_strs]
    for i in range(len(vals)):
        plt.scatter([rules[i] * a for a in attrs], vals[i], label=r_strs[i])

    plt.xscale(x_scale)
    plt.yscale(y_scale)
    plt.grid(True)

    plt.xlabel(x_axis)
    plt.ylabel(y_axis)
    if title != "":
        plt.title(title)
    plt.legend()

    plt.savefig("graphs/" + fname, dpi=300, bbox_inches="tight")
    plt.clf()


def draw_actual_line_graph(
    fname,
    vals,
    r_strs,
    a_strs,
    x_axis,
    y_axis,
    title="",
    x_scale="linear",
    y_scale="linear",
):
    rules = [int(x.split()[0]) for x in r_strs]
    attrs = [int(x.split()[0]) for x in a_strs]
    x = []
    for i in range(len(vals)):
        x = [rules[i] * a for a in attrs]
        plt.scatter(x, vals[i], label=r_strs[i])

    z = np.polyfit(x, vals[0], 1)
    plt.xscale(x_scale)
    plt.yscale(y_scale)
    plt.grid(True)

    p = np.poly1d(z)
    plt.plot(x, p(x), color="gray")
    plt.xlabel(x_axis)
    plt.ylabel(y_axis)
    if title != "":
        plt.title(title)

    plt.savefig("graphs/" + fname, dpi=300, bbox_inches="tight")
    plt.clf()


def draw_line_graph(
    fname,
    vals,
    r_strs,
    a_strs,
    x_axis,
    y_axis,
    title="",
    x_scale="linear",
    y_scale="linear",
):
    rules = [int(x.split()[0]) for x in r_strs]
    attrs = [int(x.split()[0]) for x in a_strs]
    for j, v in enumerate(vals):
        lbl = (str(1) if j == 0 else str(3)) + " daemon connections/enforcer"
        plt.scatter(
            [rules[i] * a for i in range(len(v)) for a in attrs],
            [v[i] for i in range(len(v))],
            label=lbl,
        )

    plt.xscale(x_scale)
    plt.yscale(y_scale)
    plt.grid(True)

    plt.xlabel(x_axis)
    plt.ylabel(y_axis)
    if title != "":
        plt.title(title)
    plt.legend()

    plt.savefig("graphs/" + fname, dpi=300, bbox_inches="tight")
    plt.clf()


def main():
    baseline_latency_dict = get_data_dict("BenchmarkBaseline.txt")
    baseline_latency_mean_dict, baseline_latency_std_dict = get_values_dict(
        baseline_latency_dict, "BenchmarkBaseline", latency=False
    )
    print("BenchmarkBaseline Latency:")
    print_tex_table(baseline_latency_mean_dict, baseline_latency_std_dict)

    baseline_throughput_dict = get_throughput_data_dict("BenchmarkBaseline.txt")
    baseline_throughput_mean_dict, baseline_throughput_std_dict = get_values_dict(
        baseline_throughput_dict, "BenchmarkBaseline", latency=False
    )
    print("BenchmarkBaseline throughput:")
    print_tex_table(baseline_throughput_mean_dict, baseline_throughput_std_dict)

    baseline_parallel_throughput_dict = get_throughput_data_dict(
        "BenchmarkBaselineParallel.txt"
    )
    baseline_parallel_throughput_mean_dict, baseline_parallel_throughput_std_dict = (
        get_values_dict(
            baseline_parallel_throughput_dict,
            "BenchmarkBaselineParallel",
            latency=False,
        )
    )
    print("BenchmarkBaselineParallel Throughput:")
    print_tex_table(
        baseline_parallel_throughput_mean_dict, baseline_parallel_throughput_std_dict
    )

    baseline_parallel_latency_dict = get_multicore_latency_data_dict(
        "BenchmarkBaselineParallel.txt", 8
    )
    baseline_parallel_latency_mean_dict, baseline_parallel_latency_std_dict = (
        get_values_dict(
            baseline_parallel_latency_dict, "BenchmarkBaselineParallel", latency=False
        )
    )
    print("BenchmarkBaselineParallel Latency:")
    print_tex_table(
        baseline_parallel_latency_mean_dict, baseline_parallel_latency_std_dict
    )

    acs_latency_dict = get_data_dict("BenchmarkACSFinal.txt")
    acs_latency_mean_dict, acs_latency_std_dict = get_values_dict(
        acs_latency_dict, "BenchmarkACS", latency=True
    )
    print("BenchmarkACS Latency:")
    print_tex_table(acs_latency_mean_dict, acs_latency_std_dict)

    acs_throughput_dict = get_throughput_data_dict("BenchmarkACSFinal.txt")
    acs_throughput_mean_dict, acs_throughput_std_dict = get_values_dict(
        acs_throughput_dict, "BenchmarkACS", latency=False
    )
    print("BenchmarkACS Throughput:")
    print_tex_table(acs_throughput_mean_dict, acs_throughput_std_dict)

    paac_no_net_latency_dict = get_data_dict("BenchmarkPAACNoNetFinal.txt")
    paac_no_net_latency_mean_dict, paac_no_net_latency_std_dict = get_values_dict(
        paac_no_net_latency_dict, "BenchmarkPAACNoNet", latency=True
    )
    print("BenchmarkPAACNoNet Latency:")
    print_tex_table(paac_no_net_latency_mean_dict, paac_no_net_latency_std_dict)

    paac_no_net_throughput_dict = get_throughput_data_dict(
        "BenchmarkPAACNoNetFinal.txt"
    )
    paac_no_net_throughput_mean_dict, paac_no_net_throughput_std_dict = get_values_dict(
        paac_no_net_throughput_dict, "BenchmarkPAACNoNet", latency=False
    )
    print("BenchmarkPAACNoNet Throughput:")
    print_tex_table(paac_no_net_throughput_mean_dict, paac_no_net_throughput_std_dict)

    paac_net_latency_dict = get_data_dict("BenchmarkPAACNetFinal.txt")
    paac_net_latency_mean_dict, paac_net_latency_std_dict = get_values_dict(
        paac_net_latency_dict, "BenchmarkPAACNet", latency=True
    )
    print("BenchmarkPAACNet Latency:")
    print_tex_table(paac_net_latency_mean_dict, paac_net_latency_std_dict)

    paac_net_throughput_dict = get_throughput_data_dict("BenchmarkPAACNetFinal.txt")
    paac_net_throughput_mean_dict, paac_net_throughput_std_dict = get_values_dict(
        paac_net_throughput_dict, "BenchmarkPAACNet", latency=False
    )
    print("BenchmarkPAACNet Throughput:")
    print_tex_table(paac_net_throughput_mean_dict, paac_net_throughput_std_dict)

    paac_no_cache_latency_vlist = []
    paac_no_cache_latency_mean_dict = {}
    print("BenchmarkPAACNoNetNoCache Latency:")
    for i in [1, 3, 5]:
        k = f"{i}Builders"
        paac_no_cache_latency_dict = get_no_cache_data_dict(
            "BenchmarkPAACNoNetNoCacheFinal.txt", k, latency=True
        )
        paac_no_cache_latency_mean_dict, paac_no_cache_latency_std_dict = (
            get_values_dict(
                paac_no_cache_latency_dict, "BenchmarkPAACNoNetNoCache", latency=True
            )
        )
        paac_no_cache_latency_vlist.append(
            get_values(paac_no_cache_latency_mean_dict)[2]
        )
        print(k + ":")
        print_tex_table(paac_no_cache_latency_mean_dict, paac_no_cache_latency_std_dict)

    paac_no_cache_throughput_vlist = []
    paac_no_cache_throughput_mean_dict = {}
    print("BenchmarkPAACNoNetNoCache Throughput:")
    for i in [1, 3, 5]:
        k = f"{i}Builders"
        paac_no_cache_throughput_dict = get_no_cache_data_dict(
            "BenchmarkPAACNoNetNoCacheFinal.txt", k, latency=False
        )
        paac_no_cache_throughput_mean_dict, paac_no_cache_throughput_std_dict = (
            get_values_dict(
                paac_no_cache_throughput_dict,
                "BenchmarkPAACNoNetNoCache",
                latency=False,
            )
        )
        paac_no_cache_throughput_vlist.append(
            get_values(paac_no_cache_throughput_mean_dict)[2]
        )
        print(k + ":")
        print_tex_table(
            paac_no_cache_throughput_mean_dict, paac_no_cache_throughput_std_dict
        )

    paac_single_path_latency_dict = get_data_dict("BenchmarkPAACSinglePathFinal.txt")
    paac_single_path_latency_mean_dict, paac_single_path_latency_std_dict = (
        get_single_path_values_dict(
            paac_single_path_latency_dict, "BenchmarkPAACSinglePath", latency=True
        )
    )
    print("BenchmarkPAACSinglePath Latency:")
    print_tex_table(
        paac_single_path_latency_mean_dict, paac_single_path_latency_std_dict
    )

    paac_single_path_throughput_dict = get_throughput_data_dict(
        "BenchmarkPAACSinglePathFinal.txt"
    )
    paac_single_path_throughput_mean_dict, paac_single_path_throughput_std_dict = (
        get_single_path_values_dict(
            paac_single_path_throughput_dict, "BenchmarkPAACSinglePath", latency=False
        )
    )
    print("BenchmarkPAACSinglePath Throughput:")
    print_tex_table(
        paac_single_path_throughput_mean_dict, paac_single_path_throughput_std_dict
    )

    # -----------------------------------------------------------------------------------------
    rules, attrs, values = get_values(baseline_throughput_mean_dict)
    draw_point_graph(
        "baseline_throughput_scatter.png",
        values,
        rules,
        attrs,
        r"Total attributes (log scale)",
        r"Throughput in requests/s (log scale)",
        x_scale="log",
        y_scale="log",
    )
    rules, attrs, values2 = get_values(baseline_parallel_throughput_mean_dict)
    draw_point_graph(
        "baseline_parallel_throughput_scatter.png",
        values2,
        rules,
        attrs,
        r"Total attributes (log scale)",
        r"Throughput in requests/s (log scale)",
        x_scale="log",
        y_scale="log",
    )

    rules, attrs, values = get_values(baseline_throughput_mean_dict)
    rules, attrs, values2 = get_values(baseline_parallel_throughput_mean_dict)
    draw_graph(
        "baseline_parallel_speedup_bars.png",
        r"speedup",
        rules,
        attrs,
        values2 / values,
        add_height=False,
        yscale="linear",
        second_color="cornflowerblue",
        main_color="orangered",
    )

    rules, attrs, values = get_values(acs_latency_mean_dict)
    draw_point_graph(
        "acs_latency_scatter.png",
        values,
        rules,
        attrs,
        r"Total attributes (log scale)",
        r"Latency in $\mu s$ (log scale)",
        x_scale="log",
        y_scale="log",
    )

    rules, attrs, values = get_values(acs_throughput_mean_dict)
    draw_point_graph(
        "acs_throughput_scatter.png",
        values,
        rules,
        attrs,
        r"Total attributes (log scale)",
        r"Throughput in requests/s (log scale)",
        x_scale="log",
        y_scale="log",
    )

    rules, attrs, values = get_values(paac_no_net_latency_mean_dict)
    draw_point_graph(
        "paac_latency_scatter.png",
        values,
        rules,
        attrs,
        r"Total attributes (log scale)",
        r"Latency in $\mu s$ (log scale)",
        x_scale="log",
        y_scale="log",
    )

    rules, attrs, values = get_values(paac_no_net_throughput_mean_dict)
    draw_point_graph(
        "paac_throughput_scatter.png",
        values,
        rules,
        attrs,
        r"Total attributes (log scale)",
        r"Throughput in requests/s (log scale)",
        x_scale="log",
        y_scale="log",
    )

    rules, attrs, values = get_values(acs_throughput_mean_dict)
    rules, attrs, values2 = get_values(paac_no_net_throughput_mean_dict)
    draw_graph(
        "paac_slowdown_bars.png",
        r"achieved throughput %",
        rules,
        attrs,
        (values / values2) * 100,
        add_height=False,
        yscale="linear",
        second_color="cornflowerblue",
        main_color="orangered",
    )

    rules, attrs, values = get_values(acs_latency_mean_dict)
    rules, attrs, values2 = get_values(paac_no_net_latency_mean_dict)
    draw_graph(
        "paac_latency_difference_bars.png",
        r"lantency difference in %",
        rules,
        attrs,
        (values2 / values) * 100 - 100,
        add_height=False,
        yscale="linear",
        second_color="orangered",
        main_color="cornflowerblue",
    )

    rules, attrs, values = get_values(acs_throughput_mean_dict)
    rules, attrs, values2 = get_values(paac_no_net_throughput_mean_dict)
    draw_graph(
        "paac_throughput_difference_bars.png",
        r"Throughput difference in %",
        rules,
        attrs,
        ((values2 / values) * 100 - 100),
        add_height=False,
        yscale="linear",
        second_color="cornflowerblue",
        main_color="orangered",
    )

    rules, attrs, _ = get_values(paac_no_cache_latency_mean_dict)
    draw_point_graph(
        "paac_no_cache_lines_latency.png",
        paac_no_cache_latency_vlist[0],
        rules,
        attrs,
        r"Total attributes (log scale)",
        r"Latency in $\mu s$ (log scale)",
        x_scale="log",
        y_scale="log",
    )

    rules, attrs, _ = get_values(paac_no_cache_throughput_mean_dict)
    draw_line_graph(
        "paac_no_cache_lines_throughput.png",
        (paac_no_cache_throughput_vlist[0], paac_no_cache_throughput_vlist[1]),
        rules,
        attrs,
        r"Total attributes (log scale)",
        r"Throughput in requests/s (log scale)",
        x_scale="log",
        y_scale="log",
    )

    rules, attrs, values = get_values(paac_single_path_latency_mean_dict)
    draw_actual_line_graph(
        "paac_single_path_latency_trend.png",
        values,
        rules,
        attrs,
        r"Path length in AS hops",
        r"Latency in $\mu s$",
        x_scale="linear",
        y_scale="linear",
    )


if __name__ == "__main__":
    main()
