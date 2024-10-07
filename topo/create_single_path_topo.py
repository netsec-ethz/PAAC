import sys


def create_topology(num_isd, isd_depth, out_file):
    with open(out_file, "w") as file:
        assert 2 <= num_isd <= 9
        assert 1 <= isd_depth <= 99
        as_list = [(isd + 1) * 100 for isd in range(num_isd)]
        print("core AS list:", as_list)
        as_list += [
            isd * 100 + asn + 1 for isd in [1, num_isd] for asn in range(isd_depth)
        ]
        print("AS list:", as_list)

        asn_dict = {asn: f"{str(asn)[0]}-ff00:0:{asn}" for asn in as_list}

        file.write("---\n")
        file.write("ASes:\n")
        core_asns = as_list[:num_isd]
        prefix = '  "'
        for core_asn in core_asns:
            file.write(
                prefix
                + asn_dict[core_asn]
                + '": {core: true, voting: true, authoritative: true, issuing: true}\n'
            )
        for asn in as_list:
            if asn in core_asns:
                continue

            cert_issuer = core_asns[int(str(asn)[0]) - 1]
            file.write(
                prefix
                + asn_dict[asn]
                + '": {cert_issuer: "'
                + asn_dict[cert_issuer]
                + '"}\n'
            )
        file.write("links:\n")
        prefix = "  - "
        for asn in as_list[1:]:
            as2 = asn
            if asn in core_asns:
                as1 = asn - 100
                file.write(
                    prefix
                    + '{a: "'
                    + asn_dict[as1]
                    + '", b: "'
                    + asn_dict[as2]
                    + '", linkAtoB: CORE}\n'
                )
            else:
                as1 = asn - 1
                file.write(
                    prefix
                    + '{a: "'
                    + asn_dict[as1]
                    + '", b: "'
                    + asn_dict[as2]
                    + '", linkAtoB: CHILD}\n'
                )


# This file can be used to generate a single-path topology consisting of up/down segments
# of up to 99 ASes and a core segment consisting of up to 9 core ASes in 9 different ISDs
# The resulting topology can be used to simulate long paths in a realistic network setting


def main():
    if len(sys.argv) != 4:
        print(
            "Usage: python3 create_single_path_topology.py <output .topo file> <numer of ISDs> <depth of ISDs>"
            # example: python3 create_single_path_topo.py single_path_test.topo 3 10
        )
        sys.exit(1)

    out_file = sys.argv[1]
    num_isd = int(sys.argv[2])
    isd_depth = int(sys.argv[3])

    print("Generating topology file...")
    create_topology(num_isd, isd_depth, out_file)
    print("done")


if __name__ == "__main__":
    main()
